package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newSessionsCmd returns the "sessions" sub-command stub.
//
// Real behaviour ships in S3 (sessions listing via HTTP).
func newSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sessions",
		Short: "List recent sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented in S1 — coming in S3")
			os.Exit(2)
			return nil
		},
	}
}
