package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newRunCmd returns the "run" sub-command stub.
//
// Real behaviour ships in S6 once ADR-005 (run-semantics: foreground vs.
// detached daemon) is approved by Admin.
func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Manage the tracelab-hub daemon (placeholder, ADR-005)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented yet — coming in S6")
			os.Exit(2)
			return nil
		},
	}
}
