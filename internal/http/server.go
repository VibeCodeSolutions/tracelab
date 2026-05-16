// Package http provides the chi-based HTTP API for the tracelab hub.
//
// Public surface is intentionally small: New constructs an http.Handler
// wired to the store, with bearer-auth, structured slog logging, panic
// recovery, request-id propagation and a server-wide timeout.
package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/VibeCodeSolutions/tracelab/internal/dashboard"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// Config controls runtime parameters of the HTTP layer that are not
// already covered by the chi defaults.
type Config struct {
	// AuthToken is the shared secret expected in `Authorization: Bearer <token>`.
	// An empty token disables auth — this is rejected by New() to avoid
	// accidentally opening up the API.
	AuthToken string

	// ReadTimeout / WriteTimeout are forwarded to the *http.Server by the caller;
	// kept here so all knobs travel together.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// Logger is the slog handler used by the request logger middleware. If nil,
	// slog.Default() is used.
	Logger *slog.Logger

	// Hub is the WebSocket pub/sub fan-out for /tail. If nil, /tail is not
	// registered and /ingest skips the broadcast step.
	Hub *ws.Hub

	// ADBManager is the bridge lifecycle coordinator for the /adb/* HTTP
	// endpoints (POST /adb/start, POST /adb/stop, GET /adb/devices). When
	// nil, the /adb/* routes are not registered — the hub then behaves
	// exactly like a pre-S5 build.
	ADBManager ADBManager

	// ADBDeviceLister enumerates attached adb devices for GET /adb/devices.
	// Typically the package-level function adb.Devices, wrapped in an
	// adapter (see cmd/hub/main.go). When nil, GET /adb/devices is not
	// registered — POST /adb/start and POST /adb/stop may still be wired
	// up via ADBManager alone, but device discovery is unavailable.
	ADBDeviceLister ADBDeviceLister

	// Dashboard is the dashboard handler bundle from internal/dashboard
	// (Phase 2c, ADR-011 Decision 4). When non-nil, /dashboard and
	// /dashboard/tab/{slug} and /dashboard/static/* are registered as a
	// dedicated sub-router. When nil, the dashboard routes are omitted
	// — production hubs always pass a non-nil handler (cmd/hub/main.go),
	// HTTP-layer unit tests that don't exercise the dashboard pass nil.
	//
	// AUTH POSTURE: the dashboard sub-router is registered WITHOUT
	// bearer middleware — **permanently Loopback-only by Admin-Confirm
	// 2026-05-16** (ADR-011 *Consequences*, post-#026 update). Browsers
	// cannot attach an Authorization header to <script src=…> or page
	// loads, and the README's explicit "no token in query string" rule
	// rules out the trivial query-token shortcut. The decision is not
	// a TODO: the operational assumption is the Phase-1 default bind
	// 127.0.0.1:<port> (single-user dev host); the dashboard inherits
	// that binding. No cookie-wrap, no reverse-proxy, no short-lived
	// session-token layer is planned. Should a future deployment need
	// remote dashboard access, that is a separate ADR (e.g. ADR-XYZ
	// Cookie-Wrap), explicitly *not* a follow-up to ADR-011.
	Dashboard *dashboard.Handler
}

// ADBManager is re-exported from internal/adb at the HTTP-layer boundary
// so server.New takes a small typed interface rather than the full
// *adb.BridgeManager. Production callers pass an *adb.BridgeManager (which
// satisfies this interface); tests inject a fake.
type ADBManager = adbBridgeManager

// ADBDeviceLister is re-exported similarly to ADBManager — see above.
type ADBDeviceLister = adbDeviceLister

// New constructs the chi router with the full middleware stack and routes
// wired to the given store.
//
// Returns nil if cfg.AuthToken is empty — callers must surface this as an
// error before serving traffic.
func New(st *store.Store, cfg Config) http.Handler {
	if cfg.AuthToken == "" {
		return nil
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	r := chi.NewRouter()

	// Order matters: RequestID first so all subsequent middlewares (and our
	// logger) can attach it; Recoverer wraps everything below so panics
	// don't kill the process; our slog logger replaces chi's stdlib one.
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(slogRequestLogger(logger))

	h := &handlers{store: st, hub: cfg.Hub, log: logger}

	// /healthz is intentionally outside the auth group.
	r.Get("/healthz", h.healthz)

	// /dashboard* — Phase 2c S1 (ADR-011). Registered outside the bearer
	// group; see the Dashboard field doc on Config for the auth-posture
	// rationale and the follow-up ADR pointer. Three routes:
	//   GET /dashboard               — full layout (HTML)
	//   GET /dashboard/tab/{slug}    — single tab body for htmx swap
	//   GET /dashboard/static/*      — embedded JS / CSS
	if cfg.Dashboard != nil {
		r.Get("/dashboard", cfg.Dashboard.LayoutHandler)
		r.Get("/dashboard/tab/*", cfg.Dashboard.TabHandler)
		r.Get("/dashboard/static/*", cfg.Dashboard.StaticHandler)
	}

	// /tail is auth-protected (registered in the pr group below) but must
	// NOT be wrapped by middleware.Timeout — chi's Timeout uses
	// http.TimeoutHandler, which is incompatible with hijacked connections
	// (websocket upgrades). So we register the timeout middleware in a
	// nested sub-group that excludes /tail.
	r.Group(func(pr chi.Router) {
		pr.Use(bearerAuth(cfg.AuthToken))
		if cfg.Hub != nil {
			pr.Get("/tail", ws.Handler(cfg.Hub, logger))
		}
		pr.Group(func(tr chi.Router) {
			tr.Use(middleware.Timeout(30 * time.Second))
			tr.Post("/session/start", h.sessionStart)
			tr.Post("/session/end", h.sessionEnd)
			tr.Post("/ingest", h.ingest)
			tr.Get("/sessions", h.listSessions)

			// /events serves the forward-cursor read for Phase-2b S4
			// (ADR-008). Sits in the bearer-guarded, 30s-timeout
			// group — same envelope as the other JSON read endpoints.
			tr.Get("/events", h.listEvents)

			// /crashes serves the session-scoped crash digest for
			// Phase-2b S6 (ADR-009). Newest-first list read (not a
			// forward cursor); shares the bearer + 30s-timeout
			// envelope with /events and /sessions.
			tr.Get("/crashes", h.listCrashes)

			// /adb/* routes — additive in S5 (ADR-004 Option B).
			// Registered inside the same timeout-bounded group as the
			// other JSON endpoints: GET /adb/devices shells out to
			// `adb devices -l` which can wedge if the local adb server
			// is hung, so a 30s wall-clock cap is the right safety net.
			// POST /adb/start kicks off a manager goroutine and returns
			// immediately, so it sits well under the timeout. POST
			// /adb/stop blocks on bridge teardown — bounded by the
			// underlying ctx-cancel + 2s detached flush in bridge.go.
			adbH := &adbHandlers{
				lister:  cfg.ADBDeviceLister,
				manager: cfg.ADBManager,
			}
			if cfg.ADBDeviceLister != nil {
				tr.Get("/adb/devices", adbH.devicesHandler)
			}
			if cfg.ADBManager != nil {
				tr.Post("/adb/start", adbH.startHandler)
				tr.Post("/adb/stop", adbH.stopHandler)
			}
		})
	})

	return r
}
