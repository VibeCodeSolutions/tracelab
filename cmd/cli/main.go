// Command tracelab is the Tracelab CLI.
//
// Phase 2a / Stage 1: skeleton with cobra root + sub-command stubs.
// All sub-commands print a "not implemented" message and exit 2; real
// behaviour lands in S2 (tail), S3 (sessions), S4 (run), S5 (adb).
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

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "tracelab",
		Short:         "Tracelab CLI — consume hub HTTP/WS API",
		Long:          "Tracelab CLI — consume the tracelab-hub HTTP/WS API for sessions, live tail, ADB control.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newRunCmd())
	root.AddCommand(newTailCmd())
	root.AddCommand(newSessionsCmd())
	root.AddCommand(newADBCmd())
	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "tracelab:", err)
		os.Exit(1)
	}
}
