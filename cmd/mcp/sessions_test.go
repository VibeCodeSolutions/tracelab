package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// callSessionsList drives the sessions_list handler closure with a
// constructed CallToolRequest. The MCP server's dispatch path (which
// calls handleToolCall) is not exercised — that path is exhaustively
// tested upstream in mcp-go. We test the handler directly so failure
// messages name the tool semantics, not the transport.
func callSessionsList(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := newSessionsListTool(c)
	req := mcp.CallToolRequest{}
	req.Params.Name = sessionsListToolName
	req.Params.Arguments = args
	res, err := tool.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if res == nil {
		t.Fatal("handler returned nil result")
	}
	return res
}

// decodeSessionsBody extracts the JSON-encoded {"sessions": [...]}
// payload from a tool result's first TextContent and returns the parsed
// envelope. Fatals on any deviation so the caller's assertions stay
// crisp.
func decodeSessionsBody(t *testing.T, res *mcp.CallToolResult) sessionsListResult {
	t.Helper()
	if res.IsError {
		t.Fatalf("expected success, got IsError=true: %v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T, want mcp.TextContent", res.Content[0])
	}
	var out sessionsListResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("decode body %q: %v", tc.Text, err)
	}
	return out
}

// errorText extracts the text content from an IsError tool result.
// Fatals on any other shape.
func errorText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("expected IsError=true, got success: %v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatal("error result has no content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("error content[0] type = %T, want mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

// TestSessionsListToolRegistered confirms the real tool replaces the S1
// sessions_stub: the server's tool registry contains sessions_list, and
// crucially does NOT contain sessions_stub anymore.
func TestSessionsListToolRegistered(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	tools := s.ListTools()

	if _, ok := tools["sessions_list"]; !ok {
		t.Errorf("sessions_list missing from registry; got %v", toolNames(tools))
	}
	if _, ok := tools["sessions_stub"]; ok {
		t.Errorf("sessions_stub should have retired in S3 but is still registered")
	}
}

// TestSessionsListDescriptionPresent guards the short description is
// non-empty and mentions the key semantics (limit + since), so the
// tools/list UX is informative without consulting docs.
func TestSessionsListDescriptionPresent(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	st := s.ListTools()["sessions_list"]
	if st == nil {
		t.Fatal("sessions_list not registered")
	}
	desc := strings.TrimSpace(st.Tool.Description)
	if desc == "" {
		t.Fatal("sessions_list has empty Description")
	}
	for _, want := range []string{"limit", "since"} {
		if !strings.Contains(strings.ToLower(desc), want) {
			t.Errorf("description %q does not mention %q", desc, want)
		}
	}
}

// TestSessionsListInputSchemaAccepts exercises the four argument-shape
// combinations admitted by ADR-007: empty, limit-only, since-only, and
// both. mcp-go v0.45.0 does NOT run JSON-schema validation in its
// server-side dispatch (verified in handleToolCall, server/server.go:1437),
// so "accepted" means the handler does not surface a Go error.
func TestSessionsListInputSchemaAccepts(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[]}`))
	})
	c := newTestHubServer(t, h)

	cases := []struct {
		name string
		args map[string]any
	}{
		{"empty", map[string]any{}},
		{"limit only", map[string]any{"limit": float64(10)}},
		{"since only", map[string]any{"since": "2026-05-15T00:00:00Z"}},
		{"limit and since", map[string]any{"limit": float64(5), "since": "2026-05-15T00:00:00Z"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callSessionsList(t, c, tc.args)
			if res.IsError {
				t.Errorf("unexpected error result: %s", errorText(t, res))
			}
		})
	}
}

// TestSessionsListInputSchemaWrongTypesTolerated documents the v0.45.0
// behaviour: mcp-go does not strict-validate input types at the dispatch
// layer (no jsonschema validator wired into server.handleToolCall), so a
// string-where-number is silently coerced by GetInt to its default (0)
// and the handler returns a successful empty result. Future tightening
// (mcp-go upgrade or local validator) would flip this — kept as a
// tripwire test rather than removed.
func TestSessionsListInputSchemaWrongTypesTolerated(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrong-typed `limit` falls through GetInt's default; the hub
		// receives no `?limit=` query string.
		if got := r.URL.RawQuery; got != "" {
			t.Errorf("expected no query, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[]}`))
	})
	c := newTestHubServer(t, h)
	res := callSessionsList(t, c, map[string]any{"limit": "not-a-number"})
	if res.IsError {
		t.Errorf("expected success (type coerced to default), got error: %s", errorText(t, res))
	}
}

// TestSessionsListHandlerCallsHub exercises the happy path end-to-end:
// the handler invokes client.ListSessions against a httptest hub, the
// hub sees the bearer header and the resolved limit, and the result
// envelope carries the hub's response verbatim.
func TestSessionsListHandlerCallsHub(t *testing.T) {
	t.Parallel()
	var gotAuth string
	var gotLimit string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sessions" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[
			{"id":"a","label":"first","started_at":1700000000000000000,"ended_at":1700000001000000000},
			{"id":"b","label":"open","started_at":1700000002000000000}
		]}`))
	})
	c := newTestHubServer(t, h)

	res := callSessionsList(t, c, map[string]any{"limit": float64(10)})
	body := decodeSessionsBody(t, res)

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token", gotAuth)
	}
	if gotLimit != "10" {
		t.Errorf("limit query = %q, want 10", gotLimit)
	}
	if len(body.Sessions) != 2 {
		t.Fatalf("got %d sessions, want 2: %+v", len(body.Sessions), body.Sessions)
	}
	if body.Sessions[0].ID != "a" || body.Sessions[0].EndedAt == nil {
		t.Errorf("session[0] unexpected: %+v", body.Sessions[0])
	}
	if body.Sessions[1].EndedAt != nil {
		t.Errorf("session[1] EndedAt should be nil (running), got %v", *body.Sessions[1].EndedAt)
	}
}

// TestSessionsListSinceFilter verifies the client-side since-filter:
// the hub returns three sessions with distinct started_at values, the
// tool is invoked with since at the median timestamp, and only sessions
// at-or-after the cutoff appear in the result envelope.
func TestSessionsListSinceFilter(t *testing.T) {
	t.Parallel()
	// 2026-05-15T12:00:00Z is the cutoff; 11:00 is filtered out,
	// 12:00 + 13:00 pass. Timestamps verified via time.Parse below.
	const (
		nsBefore = int64(1778842800000000000) // 2026-05-15T11:00:00Z
		nsAt     = int64(1778846400000000000) // 2026-05-15T12:00:00Z
		nsAfter  = int64(1778850000000000000) // 2026-05-15T13:00:00Z
	)
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Hub returns all three; client-side since-filter prunes.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[
			{"id":"old","label":"x","started_at":1778842800000000000},
			{"id":"at","label":"x","started_at":1778846400000000000},
			{"id":"new","label":"x","started_at":1778850000000000000}
		]}`))
	})
	c := newTestHubServer(t, h)

	res := callSessionsList(t, c, map[string]any{
		"since": "2026-05-15T12:00:00Z",
	})
	body := decodeSessionsBody(t, res)

	if len(body.Sessions) != 2 {
		t.Fatalf("got %d sessions after since-filter, want 2: %+v", len(body.Sessions), body.Sessions)
	}
	ids := []string{body.Sessions[0].ID, body.Sessions[1].ID}
	if ids[0] != "at" || ids[1] != "new" {
		t.Errorf("filtered IDs = %v, want [at new]", ids)
	}
	// Quick sanity: timestamps match what the hub sent — the filter
	// must not mutate StartedAt.
	if body.Sessions[0].StartedAt != nsAt || body.Sessions[1].StartedAt != nsAfter {
		t.Errorf("StartedAt drift: got [%d %d], want [%d %d]",
			body.Sessions[0].StartedAt, body.Sessions[1].StartedAt, nsAt, nsAfter)
	}
	// nsBefore must NOT appear.
	for _, s := range body.Sessions {
		if s.StartedAt == nsBefore {
			t.Errorf("nsBefore (%d) leaked past since-filter: %+v", nsBefore, s)
		}
	}
}

// TestSessionsListAuthFail asserts a 401 from the hub surfaces as a
// tool-result error (IsError=true) carrying the unauthorized hint. The
// transport-side Go error stays inside the handler — MCP callers see a
// useful message in the tool result.
func TestSessionsListAuthFail(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c := newTestHubServer(t, h)

	res := callSessionsList(t, c, map[string]any{})
	msg := errorText(t, res)
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error message %q missing 'unauthorized' marker", msg)
	}
}

// TestSessionsListInvalidSince asserts an unparseable since value fails
// fast inside the handler with a tool-result error — no hub round-trip
// is attempted, so the test uses a 500-by-default hub to prove the
// network is not touched.
func TestSessionsListInvalidSince(t *testing.T) {
	t.Parallel()
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, `nope`, http.StatusInternalServerError)
	})
	c := newTestHubServer(t, h)

	res := callSessionsList(t, c, map[string]any{"since": "not-a-date"})
	msg := errorText(t, res)
	if !strings.Contains(msg, "invalid since") {
		t.Errorf("error message %q missing 'invalid since' marker", msg)
	}
	if called {
		t.Error("hub was contacted despite invalid since; expected fail-fast")
	}
}

// TestSessionsListEmptyResultEmitsArray asserts an empty hub response
// renders as `{"sessions":[]}` not `{"sessions":null}` — JSON consumers
// (LLMs included) should never need to special-case null vs. empty.
func TestSessionsListEmptyResultEmitsArray(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[]}`))
	})
	c := newTestHubServer(t, h)

	res := callSessionsList(t, c, map[string]any{})
	if res.IsError {
		t.Fatalf("unexpected error: %s", errorText(t, res))
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T", res.Content[0])
	}
	if !strings.Contains(tc.Text, `"sessions":[]`) {
		t.Errorf("expected sessions:[] in body, got %q", tc.Text)
	}
}

// toolNames extracts a sorted list of tool names from a server's tool
// registry — used in TestSessionsListToolRegistered for clean failure
// messages.
func toolNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
