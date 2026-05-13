package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newADBCmd returns the "adb" sub-command stub.
//
// Real behaviour ships in S5 once ADR-004 (adb-scope: hub-endpoint vs.
// local subprocess) is approved by Admin.
func newADBCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adb",
		Short: "ADB-related commands (placeholder, ADR-004)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented in S1 — coming in S5")
			os.Exit(2)
			return nil
		},
	}
}
