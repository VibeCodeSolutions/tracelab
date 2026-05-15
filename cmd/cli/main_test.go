package main

import (
	"sort"
	"testing"
)

// TestRootCommandRegistersAllSubCommands is a structural smoke test: it
// builds the root command and asserts that each sub-command is registered
// by name. `run` was dropped at the Phase-2a-Closure per ADR-005 = Option C
// (CLI is a pure consumer; `tracelab-hub` is the daemon entrypoint).
func TestRootCommandRegistersAllSubCommands(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	if got, want := root.Use, "tracelab"; got != want {
		t.Fatalf("root.Use = %q, want %q", got, want)
	}

	want := []string{"adb", "sessions", "tail"}

	got := make([]string, 0, len(root.Commands()))
	for _, c := range root.Commands() {
		got = append(got, c.Name())
	}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("sub-command count = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sub-command[%d] = %q, want %q (full got=%v)", i, got[i], want[i], got)
		}
	}
}

// TestRootCommandShortDescriptionsPresent guards against accidentally
// shipping a sub-command without a Short line — cobra renders the empty
// string in --help which would silently regress the help UX.
func TestRootCommandShortDescriptionsPresent(t *testing.T) {
	t.Parallel()
	for _, c := range newRootCmd().Commands() {
		if c.Short == "" {
			t.Errorf("sub-command %q has empty Short description", c.Name())
		}
	}
}
