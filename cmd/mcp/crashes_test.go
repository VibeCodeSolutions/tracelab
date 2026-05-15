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

// callCrashesList drives the crashes_list handler closure with a
// constructed CallToolRequest. Mirrors callSessionsList /
// callTailSince — we test the handler directly so failure messages
// name the tool semantics, not the transport.
func callCrashesList(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := newCrashesListTool(c)
	req := mcp.CallToolRequest{}
	req.Params.Name = crashesListToolName
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

// decodeCrashesBody extracts the JSON-encoded {"crashes":[...]}
// envelope from a tool result's first TextContent.
func decodeCrashesBody(t *testing.T, res *mcp.CallToolResult) crashesListResult {
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
	var out crashesListResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("decode body %q: %v", tc.Text, err)
	}
	return out
}

// TestCrashesListToolRegistered confirms crashes_list replaces the S1
// crashes_stub: the server's tool registry contains crashes_list, and
// crucially does NOT contain crashes_stub anymore.
func TestCrashesListToolRegistered(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	tools := s.ListTools()
	if _, ok := tools["crashes_list"]; !ok {
		t.Errorf("crashes_list missing from registry; got %v", toolNames(tools))
	}
	if _, ok := tools["crashes_stub"]; ok {
		t.Errorf("crashes_stub should have retired in S6 but is still registered")
	}
}

// TestCrashesListDescriptionPresent guards the short description is
// non-empty and mentions the two key knobs (session_id/limit).
func TestCrashesListDescriptionPresent(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	st := s.ListTools()["crashes_list"]
	if st == nil {
		t.Fatal("crashes_list not registered")
	}
	desc := strings.TrimSpace(st.Tool.Description)
	if desc == "" {
		t.Fatal("crashes_list has empty Description")
	}
	for _, want := range []string{"session_id", "limit"} {
		if !strings.Contains(strings.ToLower(desc), want) {
			t.Errorf("description %q does not mention %q", desc, want)
		}
	}
}

// TestCrashesListInputSchemaAccepts exercises the canonical argument
// shapes per ADR-007 + ADR-009: session_id only, with limit. mcp-go
// v0.45.0 does not validate at dispatch, so "accepted" means the
// handler does not surface a Go error.
func TestCrashesListInputSchemaAccepts(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crashes":[]}`))
	})
	c := newTestHubServer(t, h)
	cases := []struct {
		name string
		args map[string]any
	}{
		{"session only", map[string]any{"session_id": "s1"}},
		{"with limit", map[string]any{"session_id": "s1", "limit": float64(50)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callCrashesList(t, c, tc.args)
			if res.IsError {
				t.Errorf("unexpected error result: %s", errorText(t, res))
			}
		})
	}
}

// TestCrashesListInputSchemaWrongTypesTolerated documents the mcp-go
// v0.45.0 behaviour: string-where-number is silently coerced by
// GetInt to its default (0). Tripwire test, fires automatically if
// mcp-go gains strict input validation.
func TestCrashesListInputSchemaWrongTypesTolerated(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrong-typed limit falls through GetInt's default; the
		// hub receives no `limit` query parameter.
		if r.URL.Query().Get("limit") != "" {
			t.Errorf("expected no limit query, got %q", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crashes":[]}`))
	})
	c := newTestHubServer(t, h)
	res := callCrashesList(t, c, map[string]any{
		"session_id": "s1",
		"limit":      "not-a-number",
	})
	if res.IsError {
		t.Errorf("expected success (type coerced to default), got: %s", errorText(t, res))
	}
}

// TestCrashesListMissingSessionFailsFast asserts that an absent or
// empty `session_id` argument fails inside the handler with a
// tool-result error — no hub round-trip is attempted.
func TestCrashesListMissingSessionFailsFast(t *testing.T) {
	t.Parallel()
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, `nope`, http.StatusInternalServerError)
	})
	c := newTestHubServer(t, h)

	for _, args := range []map[string]any{
		{},                     // session_id absent
		{"session_id": ""},     // empty session_id
		{"limit": float64(10)}, // session_id absent, other args present
	} {
		res := callCrashesList(t, c, args)
		msg := errorText(t, res)
		if !strings.Contains(msg, "session_id required") {
			t.Errorf("args %v: error %q missing 'session_id required'", args, msg)
		}
	}
	if called {
		t.Error("hub was contacted despite missing session_id — expected fail-fast")
	}
}

// TestCrashesListHandlerCallsHub exercises the happy path: the
// handler invokes client.CrashesList against a httptest hub, the hub
// sees the bearer header and the canonical query string, and the
// result envelope carries the hub's response verbatim.
func TestCrashesListHandlerCallsHub(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath, gotQuery string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"crashes":[
				{"id":42,"session_id":"s1","ts":1700000002,"fingerprint":"fp-2","stacktrace":"trace 2","count":3},
				{"id":17,"session_id":"s1","ts":1700000001,"fingerprint":"fp-1","stacktrace":"trace 1","count":1}
			]
		}`))
	})
	c := newTestHubServer(t, h)

	res := callCrashesList(t, c, map[string]any{
		"session_id": "s1",
		"limit":      float64(10),
	})
	body := decodeCrashesBody(t, res)

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization=%q", gotAuth)
	}
	if gotPath != "/crashes" {
		t.Errorf("path=%q, want /crashes", gotPath)
	}
	if gotQuery != "limit=10&session=s1" {
		t.Errorf("query=%q, want canonical limit=10&session=s1", gotQuery)
	}
	if len(body.Crashes) != 2 {
		t.Fatalf("len=%d, want 2", len(body.Crashes))
	}
	if body.Crashes[0].ID != 42 || body.Crashes[1].ID != 17 {
		t.Errorf("IDs=[%d %d], want [42 17] (newest first)",
			body.Crashes[0].ID, body.Crashes[1].ID)
	}
	if body.Crashes[0].Count != 3 {
		t.Errorf("count[0]=%d, want 3 (dedup-bumped)", body.Crashes[0].Count)
	}
}

// TestCrashesListLimitForwardedToHub asserts that the `limit` argument
// passed to the tool actually reaches the hub as a query parameter
// (regression guard: a refactor that drops the wiring would still
// pass HandlerCallsHub if the test only checks happy-path content).
func TestCrashesListLimitForwardedToHub(t *testing.T) {
	t.Parallel()
	var gotQuery string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crashes":[]}`))
	})
	c := newTestHubServer(t, h)
	_ = callCrashesList(t, c, map[string]any{
		"session_id": "s1",
		"limit":      float64(7),
	})
	if !strings.Contains(gotQuery, "limit=7") {
		t.Errorf("hub query=%q missing limit=7", gotQuery)
	}
}

// TestCrashesListAuthFail asserts a 401 from the hub surfaces as a
// tool-result error carrying the unauthorized hint. Mirrors the
// sessions_list / tail_since auth-fail tests.
func TestCrashesListAuthFail(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c := newTestHubServer(t, h)
	res := callCrashesList(t, c, map[string]any{"session_id": "s1"})
	msg := errorText(t, res)
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error %q missing 'unauthorized'", msg)
	}
}

// TestCrashesListEmptyResultEmitsArray asserts an empty hub response
// renders as `{"crashes":[]}` not `{"crashes":null}` — JSON consumers
// (LLMs included) should never need to special-case null vs. empty.
func TestCrashesListEmptyResultEmitsArray(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crashes":[]}`))
	})
	c := newTestHubServer(t, h)
	res := callCrashesList(t, c, map[string]any{"session_id": "s1"})
	if res.IsError {
		t.Fatalf("unexpected error: %s", errorText(t, res))
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T", res.Content[0])
	}
	if !strings.Contains(tc.Text, `"crashes":[]`) {
		t.Errorf("expected crashes:[] in body, got %q", tc.Text)
	}
}
