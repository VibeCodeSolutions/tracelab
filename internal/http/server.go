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
	r.Use(middleware.Timeout(30 * time.Second))

	h := &handlers{store: st, log: logger}

	// /healthz is intentionally outside the auth group.
	r.Get("/healthz", h.healthz)

	r.Group(func(pr chi.Router) {
		pr.Use(bearerAuth(cfg.AuthToken))
		pr.Post("/session/start", h.sessionStart)
		pr.Post("/session/end", h.sessionEnd)
		pr.Post("/ingest", h.ingest)
		pr.Get("/sessions", h.listSessions)
	})

	return r
}
