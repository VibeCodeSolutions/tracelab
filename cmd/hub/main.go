// Command tracelab-hub is the Tracelab daemon.
//
// Phase 1 / Stage 1: minimal skeleton. Reads a config path from --config,
// logs start/stop via slog (JSON to stderr), waits for SIGINT/SIGTERM,
// then exits cleanly. HTTP/WS/SQLite/adb wiring lands in S2-S5.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0-dev"

func main() {
	configPath := flag.String("config", "tracelab.toml", "path to tracelab.toml")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("tracelab-hub starting",
		slog.String("version", version),
		slog.String("config", *configPath),
	)

	// Block until signal. Future stages add HTTP/WS server lifecycles here,
	// each owning a goroutine cancelled by ctx.
	<-ctx.Done()

	reason := "context done"
	if cause := context.Cause(ctx); cause != nil {
		reason = cause.Error()
	}
	logger.Info("tracelab-hub stopping", slog.String("reason", reason))
}
