// Command tracelab-hub is the Tracelab daemon.
//
// Phase 2a / Stage 5: chi-based HTTP server with bearer auth, /tail WS
// fan-out, and hub-managed adb bridges (ADR-004 Option B). Lifecycle:
// load config → open store → create ws hub → create adb bridge manager →
// optionally auto-start one bridge from [adb] config → start http.Server
// in a goroutine → wait for SIGINT/SIGTERM → graceful shutdown (adb
// bridges → ws hub → http server) → close store.
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
	"github.com/VibeCodeSolutions/tracelab/internal/agents"
	"github.com/VibeCodeSolutions/tracelab/internal/config"
	"github.com/VibeCodeSolutions/tracelab/internal/dashboard"
	httplayer "github.com/VibeCodeSolutions/tracelab/internal/http"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0-dev"

// adbDeviceListerFunc adapts a plain `func(ctx) ([]adb.Device, error)` to
// the httplayer.ADBDeviceLister interface. Used to point the HTTP layer at
// the package-level adb.Devices function without standing up a wrapper
// struct in cmd/hub.
type adbDeviceListerFunc func(ctx context.Context) ([]adb.Device, error)

func (f adbDeviceListerFunc) Devices(ctx context.Context) ([]adb.Device, error) {
	return f(ctx)
}

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

	// adb bridge manager + device-lister adapter feed the new /adb/* HTTP
	// endpoints (S5, ADR-004 Option B). Both are wired unconditionally —
	// they cost nothing when no client touches the endpoints, and they
	// let the CLI / future MCP server drive the bridges over HTTP.
	adbMgr := adb.NewBridgeManager(adb.BridgeManagerDeps{
		Store:  st,
		Hub:    hub,
		Logger: logger,
	})
	defer adbMgr.Close()

	// Dashboard handler (Phase 2c S1, ADR-011). Template-parse failures
	// surface here as a fatal start-up error — we refuse to come up with
	// a broken UI rather than 500-ing on the first dashboard hit. The
	// store is threaded through so the Phase 2c S3 data-driven tabs
	// (sessions + detail view) can query it.
	dashHandler, err := dashboard.NewHandler(version, logger, st)
	if err != nil {
		return fmt.Errorf("dashboard handler: %w", err)
	}

	// Agent-observability handler bundle (Phase 2d S1, ADR-013).
	// Owns POST /agents/ingest — registered inside the bearer +
	// 30s-timeout group via httplayer.Config below.
	agentHandler := agents.NewHandler(st, logger)

	addr := cfg.Server.Bind + ":" + strconv.Itoa(cfg.Server.Port)
	handler := httplayer.New(st, httplayer.Config{
		AuthToken:       cfg.Auth.Token,
		Logger:          logger,
		Hub:             hub,
		ADBManager:      adbMgr,
		ADBDeviceLister: adbDeviceListerFunc(adb.Devices),
		Dashboard:       dashHandler,
		Agents:          agentHandler,
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

	// Optional config-driven adb-logcat bridge. Owned by the bridge
	// manager, started via adbMgr.Start so the cfg.ADB.Enabled path goes
	// through the same lifecycle as bridges launched on demand via
	// POST /adb/start. The manager owns the goroutine; we wait for its
	// teardown via adbMgr.Close in the shutdown block below — that
	// preserves the "bridge stops before hub.Close" invariant from S7.
	//
	// Empty DeviceSerial in cfg.ADB is rejected here: the manager keys
	// bridges by serial, and the legacy "let adb pick" mode doesn't
	// compose with multi-device management. Operators who relied on
	// auto-pick should set [adb].device_serial explicitly going forward
	// (single-device dev boxes can copy the value from `adb devices`).
	if cfg.ADB.Enabled {
		if cfg.ADB.DeviceSerial == "" {
			return errors.New("config: [adb].enabled=true requires [adb].device_serial (S5: bridges are keyed by serial)")
		}
		logger.Info("adb bridge enabled",
			slog.String("device_serial", cfg.ADB.DeviceSerial),
			slog.String("tag_filter", cfg.ADB.TagFilter),
		)
		if _, err := adbMgr.Start(adb.BridgeStartOptions{
			DeviceSerial: cfg.ADB.DeviceSerial,
			TagFilter:    cfg.ADB.TagFilter,
		}); err != nil {
			return fmt.Errorf("adb bridge auto-start: %w", err)
		}
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
	//   1. adb bridges stopping (publishes drained, sessions ended) — all
	//      managed bridges, regardless of whether they were auto-started
	//      via cfg.ADB or launched on demand via POST /adb/start.
	//   2. websocket hub closed (subscribers released, close-frames sent)
	//   3. http server stopped (graceful srv.Shutdown returns)
	// Reversed orders would either lose late events (hub.Close before
	// bridges stop races a Publish) or stall srv.Shutdown waiting on
	// hijacked /tail conns (srv.Shutdown before hub.Close).
	adbMgr.Close()
	logger.Info("adb bridges stopped")

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
