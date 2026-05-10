// Command tracelab-hub is the Tracelab daemon.
//
// Phase 1 / Stage 3: chi-based HTTP server with bearer auth wired to the
// SQLite store. Lifecycle: load config → open store → start http.Server in a
// goroutine → wait for SIGINT/SIGTERM → graceful shutdown (5s) → close store.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	stdhttp "net/http"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/config"
	httplayer "github.com/VibeCodeSolutions/tracelab/internal/http"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0-dev"

func main() {
	if err := run(); err != nil {
		slog.Error("tracelab-hub fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "tracelab.toml", "path to tracelab.toml")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if cfg.Auth.Token == "" || cfg.Auth.Token == "CHANGEME" {
		return errors.New("config: [auth].token must be set to a non-default value (generate via `openssl rand -hex 32`)")
	}

	if err := os.MkdirAll(cfg.Storage.DatastorePath, 0o755); err != nil {
		return fmt.Errorf("create datastore dir: %w", err)
	}
	dbPath := filepath.Join(cfg.Storage.DatastorePath, "tracelab.db")
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() {
		if err := st.Close(); err != nil {
			logger.Error("store close failed", slog.Any("error", err))
		}
	}()

	hub := ws.NewHub(0)
	defer hub.Close()

	addr := cfg.Server.Bind + ":" + strconv.Itoa(cfg.Server.Port)
	handler := httplayer.New(st, httplayer.Config{
		AuthToken:    cfg.Auth.Token,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		Logger:       logger,
		Hub:          hub,
	})
	if handler == nil {
		return errors.New("http: New returned nil (auth token empty?)")
	}
	// NOTE: WriteTimeout on the underlying http.Server is intentionally
	// not propagated here because it would also apply to long-lived
	// /tail websocket connections (after Hijack the deadline still ticks
	// on the underlying conn for the *original* response). We rely on
	// ws.PingPeriod / ws.PongWait to detect dead clients instead.
	srv := &stdhttp.Server{
		Addr:        addr,
		Handler:     handler,
		ReadTimeout: cfg.Server.ReadTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("tracelab-hub starting",
		slog.String("version", version),
		slog.String("config", *configPath),
		slog.String("db", dbPath),
	)
	logger.Info("http server listening", slog.String("addr", addr))

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("tracelab-hub stopping", slog.String("reason", "signal"))
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
	}

	// Close the WS hub before srv.Shutdown so /tail handlers send their
	// close frames and release the hijacked conns; otherwise srv.Shutdown
	// would wait for them until shutdownCtx expires.
	hub.Close()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown failed", slog.Any("error", err))
	}
	return nil
}
