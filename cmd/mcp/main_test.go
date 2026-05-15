package main

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestNewServerConstructs is a structural smoke test: it builds the MCP
// server and asserts the construction path itself doesn't panic. Phase 2b
// S1 has no behaviour to test; this guards the registration wiring only.
func TestNewServerConstructs(t *testing.T) {
	t.Parallel()
	if s := newServer(); s == nil {
		t.Fatal("newServer() returned nil")
	}
}

// TestNewServerRegistersAllStubTools asserts the four placeholder tools
// from stubTools are present in the server's tool registry by name.
// Mirrors TestRootCommandRegistersAllSubCommands in cmd/cli for the CLI
// side: registration is the only thing S1 promises.
func TestNewServerRegistersAllStubTools(t *testing.T) {
	t.Parallel()
	s := newServer()
	tools := s.ListTools()

	want := []string{"adb_stub", "crashes_stub", "sessions_stub", "tail_stub"}
	got := make([]string, 0, len(tools))
	for name := range tools {
		got = append(got, name)
	}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("tool count = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q (full got=%v)", i, got[i], want[i], got)
		}
	}
}

// TestStubToolDescriptionsPresent guards against shipping a placeholder
// without a description — analogous to TestRootCommandShortDescriptionsPresent
// in cmd/cli. An empty Description silently regresses the tools/list UX
// that human and CC operators rely on.
func TestStubToolDescriptionsPresent(t *testing.T) {
	t.Parallel()
	s := newServer()
	for name, st := range s.ListTools() {
		if strings.TrimSpace(st.Tool.Description) == "" {
			t.Errorf("tool %q has empty Description", name)
		}
	}
}

// TestStubHandlerReturnsNotImplemented asserts every S1 handler returns a
// structured "not implemented" error pointing at ADR-007. This is the
// contract S2 will replace, so we pin the marker today.
func TestStubHandlerReturnsNotImplemented(t *testing.T) {
	t.Parallel()
	res, err := stubHandler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("stubHandler returned unexpected Go error: %v", err)
	}
	if res == nil {
		t.Fatal("stubHandler returned nil result")
	}
	if !res.IsError {
		t.Errorf("stubHandler result IsError = false, want true")
	}
	if len(res.Content) == 0 {
		t.Fatal("stubHandler result has no content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("stubHandler result content[0] type = %T, want mcp.TextContent", res.Content[0])
	}
	if !strings.Contains(tc.Text, "ADR-007") {
		t.Errorf("stubHandler error text missing ADR-007 marker; got %q", tc.Text)
	}
}
