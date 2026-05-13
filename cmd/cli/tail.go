package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newTailCmd returns the "tail" sub-command stub.
//
// Real behaviour ships in S4 — depends on the HTTP client from S2 and the
// shared config plumbing from S3.
func newTailCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tail",
		Short: "Stream hub events live via WebSocket",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "not implemented in S1 — coming in S4")
			os.Exit(2)
			return nil
		},
	}
}
