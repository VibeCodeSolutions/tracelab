package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
	"github.com/VibeCodeSolutions/tracelab/internal/cliconfig"
)

// Output format identifiers — also accepted as --format values.
const (
	formatTable = "table"
	formatJSON  = "json"
)

// defaultSessionLimit is used when neither --limit nor a [cli] override
// is set. 20 keeps the table readable on a typical 80-line terminal.
const defaultSessionLimit = 20

// sessionTimeout caps the round-trip; long enough for an over-the-LAN
// hub, short enough for "is the hub up?" to fail fast.
const sessionTimeout = 5 * time.Second

// sessionsFlags bundles the per-invocation knobs for the sessions sub
// command. Held in a struct so testing helpers can build a Cmd with a
// pinned flag set without parsing argv twice.
type sessionsFlags struct {
	limit  int
	format string
}

// newSessionsCmd returns the wired "sessions" sub-command.
//
// Per ADR-002 + ADR-003, the command:
//
//   - resolves config via cliconfig.Resolve (5-step discovery + override)
//   - constructs a client.Client against the resolved URL+token
//   - lists sessions (limit defaults to 20 unless --limit overrides)
//   - renders table (default) or JSON
//
// Auth and connection errors are translated to short human-readable
// messages on stderr; no Go stack traces are surfaced to the user.
func newSessionsCmd() *cobra.Command {
	var flags sessionsFlags
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List recent sessions",
		Long: `List the most recent sessions recorded by the tracelab-hub.

Output is a tab-aligned table by default; pass --format=json for a
machine-readable array. The hub URL and bearer token are read from
tracelab.toml (discovered via the 5-step ADR-002 order) and may be
overridden via --url / --token or the TRACELAB_URL / TRACELAB_TOKEN
environment variables.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessions(cmd, flags)
		},
	}
	cmd.Flags().IntVarP(&flags.limit, "limit", "n", 0,
		"maximum number of sessions to return (default 20)")
	cmd.Flags().StringVarP(&flags.format, "format", "f", "",
		"output format: table | json (default from [cli].default_format, falls back to table)")
	return cmd
}

// runSessions is split from RunE so it is unit-testable: it reads the
// cobra-root persistent flags via cmd.Root() and returns errors instead
// of calling os.Exit.
func runSessions(cmd *cobra.Command, flags sessionsFlags) error {
	// Persistent flags live on the root; read them via cmd.Root() so a
	// caller invoking the sub-command in isolation still works.
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

	c, err := client.New(client.Config{
		BaseURL: resolved.BaseURL,
		Token:   resolved.Token,
		Timeout: sessionTimeout,
	})
	if err != nil {
		return userError(err.Error())
	}

	limit := flags.limit
	if limit <= 0 {
		limit = defaultSessionLimit
	}

	format := flags.format
	if format == "" {
		format = resolved.CLI.DefaultFormat
	}
	if format != formatTable && format != formatJSON {
		return userError(fmt.Sprintf("invalid --format %q (must be table or json)", format))
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), sessionTimeout)
	defer cancel()

	sessions, err := c.ListSessions(ctx, limit)
	if err != nil {
		return translateClientError(err, resolved)
	}

	out := cmd.OutOrStdout()
	switch format {
	case formatTable:
		return writeSessionsTable(out, sessions)
	case formatJSON:
		return writeSessionsJSON(out, sessions)
	}
	return nil // unreachable — guarded above
}

// userError is a sentinel-wrapped error type that signals "this is a
// user-facing message; do NOT print a stack trace". main() inspects this
// to decide whether to render `tracelab: <msg>` (clean) or the default
// `tracelab: <err>` (which is the same in practice, but the wrapper
// keeps the door open for richer formatting later, and documents intent).
type userErrorMsg string

func (u userErrorMsg) Error() string { return string(u) }

func userError(msg string) error { return userErrorMsg(msg) }

// translateClientError maps a *client.HTTPError or sentinel to a short,
// actionable user message. Hub URL is included to help the user verify
// they are talking to the right place.
func translateClientError(err error, resolved *cliconfig.Resolved) error {
	if errors.Is(err, client.ErrUnauthorized) {
		return userError("unauthorized — check token in tracelab.toml or TRACELAB_TOKEN")
	}
	if errors.Is(err, client.ErrServerError) {
		var he *client.HTTPError
		if errors.As(err, &he) {
			return userError(fmt.Sprintf("hub error (HTTP %d) from %s", he.Status, resolved.BaseURL))
		}
		return userError(fmt.Sprintf("hub error from %s", resolved.BaseURL))
	}
	var he *client.HTTPError
	if errors.As(err, &he) {
		return userError(fmt.Sprintf("hub responded HTTP %d for %s", he.Status, he.Endpoint))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return userError(fmt.Sprintf("timeout contacting hub at %s", resolved.BaseURL))
	}
	// Generic connection failure — surface the BaseURL but not the raw
	// transport error (which would include Go-internal noise like dial-tcp
	// addresses).
	return userError(fmt.Sprintf("cannot reach hub at %s: %v", resolved.BaseURL, err))
}

// writeSessionsTable renders the table format: ID, Label, Started,
// Ended. A running session shows "running" in the Ended column.
//
// Columns are tab-separated and rendered through text/tabwriter so they
// align on stdout even when labels vary widely in width.
func writeSessionsTable(w io.Writer, sessions []client.Session) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tLABEL\tSTARTED\tENDED")
	for _, s := range sessions {
		started := formatSessionTime(s.StartedAt)
		ended := "running"
		if s.EndedAt != nil {
			ended = formatSessionTime(*s.EndedAt)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.ID, s.Label, started, ended)
	}
	return tw.Flush()
}

// formatSessionTime renders an event timestamp (unix-nanoseconds, hub
// convention) as a short local RFC3339 — enough precision to disambiguate
// adjacent sessions in a typical test run.
func formatSessionTime(ns int64) string {
	if ns <= 0 {
		return "-"
	}
	return time.Unix(0, ns).Format(time.RFC3339)
}

// writeSessionsJSON renders the JSON format: an array of Session DTOs,
// pretty-printed with two-space indent. The DTO matches the client
// package's wire shape (see internal/client/types.go) — pointer EndedAt
// is serialised as `null` when the session is still running.
func writeSessionsJSON(w io.Writer, sessions []client.Session) error {
	// json.MarshalIndent on a nil slice produces "null"; for an empty
	// slice we want "[]" (consistent with the hub).
	if sessions == nil {
		sessions = []client.Session{}
	}
	buf, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("cli: encode json: %w", err)
	}
	if _, err := w.Write(append(buf, '\n')); err != nil {
		return err
	}
	return nil
}

// asUserError returns the message and true when err carries a userErrorMsg
// somewhere in its chain. main() uses this to decide between clean
// stderr-print and the default cobra rendering.
func asUserError(err error) (string, bool) {
	var u userErrorMsg
	if errors.As(err, &u) {
		return string(u), true
	}
	return "", false
}

