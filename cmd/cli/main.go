// Command tracelab is the Tracelab CLI.
//
// Phase 2a / Stage 3: the `sessions` sub-command is now wired end-to-end
// against the shared client (internal/client/) and the config-discovery
// helper (internal/cliconfig/). `run`, `tail`, and `adb` remain stubs;
// real behaviour lands in S4 (tail), S5 (adb), S6 (run).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X main.version=...".
// Same convention as cmd/hub so a single VERSION variable in the Makefile
// covers both binaries.
var version = "0.1.0-dev"

// newRootCmd builds the root command tree with the persistent flags that
// every authenticated sub-command shares (--config / --url / --token).
//
// Persistent flags live on the root rather than each sub-command so the
// flag-discovery layer (cliconfig.Resolve) always sees the same triple
// regardless of which sub-command parsed them.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "tracelab",
		Short:         "Tracelab CLI — consume hub HTTP/WS API",
		Long:          "Tracelab CLI — consume the tracelab-hub HTTP/WS API for sessions, live tail, ADB control.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("config", "",
		"path to tracelab.toml (overrides $TRACELAB_CONFIG and the default search order)")
	root.PersistentFlags().String("url", "",
		"hub base URL (overrides $TRACELAB_URL and [server].port/bind from tracelab.toml)")
	root.PersistentFlags().String("token", "",
		"bearer token (overrides $TRACELAB_TOKEN and [auth].token from tracelab.toml)")

	root.AddCommand(newRunCmd())
	root.AddCommand(newTailCmd())
	root.AddCommand(newSessionsCmd())
	root.AddCommand(newADBCmd())
	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// userErrorMsg (defined in sessions.go) carries an
		// already-formatted user-facing message — print it cleanly,
		// no stack trace, no Go-internal noise.
		if msg, ok := asUserError(err); ok {
			fmt.Fprintln(os.Stderr, "tracelab:", msg)
		} else {
			fmt.Fprintln(os.Stderr, "tracelab:", err)
		}
		os.Exit(1)
	}
}
