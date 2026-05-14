package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
	"github.com/VibeCodeSolutions/tracelab/internal/cliconfig"
)

// formatPlain selects the human-readable per-line renderer with ANSI
// colour on the level token. tailFormatTag is the fallback source-tag
// printed for events whose Source field is empty (see Z.267-269 in
// writeTailEvent). The JSON renderer (formatJSON) is declared in
// sessions.go and shared across sub-cmds.
const (
	formatPlain   = "plain"
	tailFormatTag = "tail"
)

// ANSI SGR escapes. Inline so the CLI keeps a stdlib-only dependency
// footprint (no fatih/color or similar). Reset is appended once per
// coloured token; the level word is the only thing that changes colour,
// the rest of the line stays default.
const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiDim    = "\x1b[2m"
)

// tailFlags bundles the per-invocation knobs for the tail sub-command,
// identical to the sessionsFlags pattern in sessions.go.
type tailFlags struct {
	session string
	format  string
}

// newTailCmd returns the wired "tail" sub-command — the live-tail consumer
// of the hub's WebSocket /tail endpoint.
//
// The hub URL, bearer token, and config-discovery rules are shared with
// `sessions` via the root's persistent flags + cliconfig.Resolve. ANSI
// colour and the per-subscriber buffer size are read from the [cli]
// section of tracelab.toml (Color: auto|always|never, TailBuffer: int).
//
// Per ADR-003, Tail itself lives in internal/client/ — this sub-command
// is the user-facing surface that wires Tail's onEvent callback to
// stdout, handles SIGINT for clean WS close-frame termination, and
// translates client errors via the same translateClientError pipeline
// the `sessions` sub-command uses (no duplicated error mapping).
func newTailCmd() *cobra.Command {
	var flags tailFlags
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Stream hub events live via WebSocket",
		Long: `Open a WebSocket subscription against the tracelab-hub's /tail endpoint
and stream events to stdout until you press Ctrl-C (or the hub closes the
stream).

Output format defaults to a colour-coded plain text line per event
(ERROR red, WARN yellow, DEBUG dim, others default). --format=json
emits NDJSON instead — one json.Marshal'd Event per line, suitable for
piping into jq or another stream consumer.

Colour is controlled by [cli].color in tracelab.toml: "auto" (default,
emits colour only when stdout is a terminal), "always", or "never". The
buffered-channel size between the WebSocket reader and the printer is
[cli].tail_buffer (default 1024).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTail(cmd, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.session, "session", "s", "",
		"session id to filter on (required)")
	cmd.Flags().StringVarP(&flags.format, "format", "f", formatPlain,
		"output format: plain | json")
	return cmd
}

// runTail is the testable core of the tail sub-command: it reads
// persistent flags, resolves the runtime config, opens the WS stream,
// fans events into a printer goroutine, and shuts everything down
// cleanly on context cancellation (SIGINT, hub close, or read error).
//
// Returns nil on graceful termination (SIGINT, hub close); returns a
// userErrorMsg-wrapped error for auth / connection / format problems so
// main() can render them without a stack trace.
func runTail(cmd *cobra.Command, flags tailFlags) error {
	if strings.TrimSpace(flags.session) == "" {
		return userError("--session is required (use `tracelab sessions` to list IDs)")
	}

	root := cmd.Root()
	cfgPath, _ := root.PersistentFlags().GetString("config")
	flagURL, _ := root.PersistentFlags().GetString("url")
	flagToken, _ := root.PersistentFlags().GetString("token")

	resolved, err := cliconfig.Resolve(cliconfig.Sources{
		FlagConfigPath: cfgPath,
		FlagURL:        flagURL,
		FlagToken:      flagToken,
	})
	if err != nil {
		return userError(err.Error())
	}

	format := flags.format
	if format == "" {
		format = formatPlain
	}
	if format != formatPlain && format != formatJSON {
		return userError(fmt.Sprintf("invalid --format %q (must be plain or json)", format))
	}

	c, err := client.New(client.Config{
		BaseURL: resolved.BaseURL,
		Token:   resolved.Token,
		// Timeout configures the embedded http.Client (used for the
		// sessions HTTP path). The WS handshake here uses its own
		// dialer-level HandshakeTimeout in internal/client/tail.go;
		// this field is kept for parity with sessions.go and is
		// effectively a no-op for the tail sub-command.
		Timeout: 10 * time.Second,
	})
	if err != nil {
		return userError(err.Error())
	}

	// Resolve colour mode once up-front. We pass a bool down because the
	// printer goroutine does not have access to *cobra.Command or os.*
	// helpers and we want the test path (no TTY) to be fully
	// deterministic.
	colorOn := resolveColorMode(resolved.CLI.Color)

	// Channel buffer comes from [cli].tail_buffer. The WebSocket reader
	// (inside client.Tail's onEvent callback) sends synchronously, so a
	// slow stdout naturally back-pressures the reader — and the hub's
	// own drop-on-full subscriber discipline takes over from there.
	// Events that arrive are printed in order under normal operation;
	// the only exception is the in-flight event during context
	// cancellation, which is dropped by the select-on-Done bail-out in
	// the onEvent callback below (expected shutdown semantics).
	bufSize := resolved.CLI.TailBuffer
	if bufSize <= 0 {
		bufSize = 1024 // mirror DefaultCLITailBuffer; ApplyDefaults already runs in Resolve
	}
	events := make(chan client.Event, bufSize)

	// Printer goroutine: ranges over events until the channel is closed,
	// then signals printerDone. Closing happens after client.Tail
	// returns, so the printer drains any in-flight events before exit.
	var printerDone sync.WaitGroup
	printerDone.Add(1)
	out := cmd.OutOrStdout()
	go func() {
		defer printerDone.Done()
		for e := range events {
			if err := writeTailEvent(out, e, format, colorOn); err != nil {
				// stdout write failure is fatal but rare (pipe closed
				// by `head` etc.); log to stderr and drain to avoid
				// deadlocking the producer.
				fmt.Fprintln(cmd.ErrOrStderr(), "tracelab: tail: write failed:", err)
				for range events { // drain
				}
				return
			}
		}
	}()

	// Signal handling — SIGINT/SIGTERM cancels the context, which fires
	// client.Tail's watcher goroutine to send a close-frame and tear the
	// conn down. Tail returns nil on user-driven cancellation.
	//
	// We use NotifyContext so the signal handler is automatically removed
	// when the parent context is done (no goroutine leak in tests where
	// runTail returns for non-signal reasons).
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tailErr := c.Tail(ctx, flags.session, func(e client.Event) {
		// Blocking send — back-pressure flows up into the WS read loop.
		// On ctx cancel the connection is closed by the watcher inside
		// client.Tail, the read loop returns, and we drop out of Tail
		// without ever blocking here forever.
		select {
		case events <- e:
		case <-ctx.Done():
		}
	})
	close(events)
	printerDone.Wait()

	if tailErr != nil {
		return translateClientError(tailErr, resolved)
	}
	return nil
}

// resolveColorMode maps the [cli].color setting into a concrete bool.
//
//   - "always" → true unconditionally
//   - "never"  → false unconditionally
//   - "auto" (or any other value, defensively) → true iff stdout is a
//     character device (i.e. a terminal); false when stdout is a pipe,
//     file, or — crucially for the test suite — a *bytes.Buffer.
//
// We probe os.Stdout directly rather than cmd.OutOrStdout(): the latter
// is a *bytes.Buffer under test (no Stat method beyond io.Writer), and
// the user-facing colour decision is about the real terminal anyway.
func resolveColorMode(mode string) bool {
	switch strings.ToLower(mode) {
	case "always":
		return true
	case "never":
		return false
	default: // "auto" and any unknown value
		return isTerminal(os.Stdout)
	}
}

// isTerminal returns true when f is a character device (TTY). Uses
// os.File.Stat + ModeCharDevice so we stay stdlib-only — no
// golang.org/x/term, no isatty dep.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}

// writeTailEvent formats a single event according to format + colorOn and
// writes it to w. The plain format is one line per event:
//
//	<ts-rfc3339> <LEVEL> [<source>] <msg>
//
// JSON format is one json.Marshal'd Event per line (NDJSON), unindented,
// terminated by '\n' so each line stands on its own — `jq -c` and
// similar consumers expect exactly this.
func writeTailEvent(w io.Writer, e client.Event, format string, colorOn bool) error {
	switch format {
	case formatJSON:
		buf, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("cli: tail: marshal event: %w", err)
		}
		_, err = fmt.Fprintln(w, string(buf))
		return err
	default: // plain
		level := strings.ToUpper(e.Level)
		levelToken := level
		if colorOn {
			levelToken = colourise(level)
		}
		ts := "-"
		if e.TS > 0 {
			ts = time.Unix(0, e.TS).Format(time.RFC3339)
		}
		source := e.Source
		if source == "" {
			source = tailFormatTag
		}
		_, err := fmt.Fprintf(w, "%s %s [%s] %s\n", ts, levelToken, source, e.Msg)
		return err
	}
}

// colourise wraps an upper-case level token in the matching ANSI SGR
// escape. ERROR/FATAL → red, WARN/WARNING → yellow, DEBUG/TRACE → dim,
// everything else → no escape (caller already passed the bare token).
//
// Reset is appended exactly once so the trailing message text stays
// default-coloured.
func colourise(level string) string {
	switch level {
	case "ERROR", "FATAL":
		return ansiRed + level + ansiReset
	case "WARN", "WARNING":
		return ansiYellow + level + ansiReset
	case "DEBUG", "TRACE":
		return ansiDim + level + ansiReset
	default:
		return level
	}
}
