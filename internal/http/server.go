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
}

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
		})
	})

	return r
}
