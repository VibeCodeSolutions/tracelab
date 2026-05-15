package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// newTestServer wires a *MCPServer against a httptest-backed hub. Tests
// drive buildServer directly so they bypass cliconfig discovery (the
// cliconfig path is exercised separately in internal/cliconfig). The
// returned http test-server is closed via t.Cleanup.
func newTestHubServer(t *testing.T, h http.Handler) *client.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := client.New(client.Config{BaseURL: srv.URL, Token: "test-token"})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

// TestBuildServerConstructs is a structural smoke test: it builds the
// MCP server against a no-op hub client and asserts construction itself
// doesn't panic.
func TestBuildServerConstructs(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if s := buildServer(c); s == nil {
		t.Fatal("buildServer() returned nil")
	}
}

// TestServerRegistersExpectedTools asserts the real sessions_list tool
// and the three remaining stub placeholders are present in the server's
// tool registry. Sorted-name comparison gives deterministic failure
// messages when a tool moves in or out.
func TestServerRegistersExpectedTools(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	tools := s.ListTools()

	// Sorted alphabetically: adb_stub, crashes_stub, sessions_list,
	// tail_stub. sessions_stub from the S1 skeleton has retired in S3.
	want := []string{"adb_stub", "crashes_stub", "sessions_list", "tail_stub"}
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

// TestToolDescriptionsPresent guards against shipping a tool without a
// description — an empty Description silently regresses the tools/list
// UX that human and CC operators rely on. Covers the real tool plus the
// remaining stubs in one sweep.
func TestToolDescriptionsPresent(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	for name, st := range s.ListTools() {
		if strings.TrimSpace(st.Tool.Description) == "" {
			t.Errorf("tool %q has empty Description", name)
		}
	}
}

// TestStubHandlerReturnsNotImplemented asserts the remaining placeholder
// handler returns a structured "not implemented" error pointing at
// ADR-007. sessions_stub retired in S3 (covered by sessions_list tests
// in sessions_test.go); the loop only walks the still-stubbed tools.
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
