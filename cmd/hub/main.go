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

	"github.com/VibeCodeSolutions/tracelab/internal/adb"
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

	// Optional adb-logcat bridge. Owned by main: started here under a
	// child context derived from ctx so we can stop the bridge *before*
	// hub.Close (otherwise its trailing Publish call would race the
	// hub teardown). bridgeDone is closed when Run returns.
	var (
		bridgeCancel context.CancelFunc
		bridgeDone   chan struct{}
	)
	if cfg.ADB.Enabled {
		br := adb.NewBridge(adb.BridgeConfig{
			DeviceSerial: cfg.ADB.DeviceSerial,
			TagFilter:    cfg.ADB.TagFilter,
			Store:        st,
			Hub:          hub,
			Logger:       logger,
		})
		var bridgeCtx context.Context
		bridgeCtx, bridgeCancel = context.WithCancel(ctx)
		// Safety net: even if a later return-path skips the explicit
		// shutdown sequence below, the deferred cancel guarantees the
		// bridge goroutine terminates. The explicit cancel+wait below
		// is what actually orders the slog messages on the happy path.
		defer bridgeCancel()
		bridgeDone = make(chan struct{})
		logger.Info("adb bridge enabled",
			slog.String("device_serial", cfg.ADB.DeviceSerial),
			slog.String("tag_filter", cfg.ADB.TagFilter),
		)
		go func() {
			defer close(bridgeDone)
			if err := br.Run(bridgeCtx); err != nil {
				logger.Error("adb bridge exited", slog.Any("error", err))
			}
		}()
	}

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

	// Shutdown ordering matters and is observable in slog:
	//   1. adb bridge stopping (publishes drained, session ended)
	//   2. websocket hub closed (subscribers released, close-frames sent)
	//   3. http server stopped (graceful srv.Shutdown returns)
	// Reversed orders would either lose late events (hub.Close before
	// bridge stops races a Publish) or stall srv.Shutdown waiting on
	// hijacked /tail conns (srv.Shutdown before hub.Close).
	if bridgeCancel != nil {
		bridgeCancel()
		<-bridgeDone
		logger.Info("adb bridge stopped")
	}

	hub.Close()
	logger.Info("websocket hub closed")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown failed", slog.Any("error", err))
	}
	logger.Info("http server stopped")
	return nil
}
