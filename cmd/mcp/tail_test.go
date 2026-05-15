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

// callTailSince drives the tail_since handler closure with a constructed
// CallToolRequest. Mirrors callSessionsList in sessions_test.go — we
// test the handler directly so failure messages name the tool
// semantics, not the transport.
func callTailSince(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := newTailSinceTool(c)
	req := mcp.CallToolRequest{}
	req.Params.Name = tailSinceToolName
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

// decodeTailBody extracts the JSON-encoded {"events":[...], "next_since_seq":...}
// envelope from a tool result's first TextContent.
func decodeTailBody(t *testing.T, res *mcp.CallToolResult) tailSinceResult {
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
	var out tailSinceResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("decode body %q: %v", tc.Text, err)
	}
	return out
}

// TestTailSinceToolRegistered confirms tail_since replaces the S1
// tail_stub: the server's tool registry contains tail_since, and
// crucially does NOT contain tail_stub anymore.
func TestTailSinceToolRegistered(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	tools := s.ListTools()
	if _, ok := tools["tail_since"]; !ok {
		t.Errorf("tail_since missing from registry; got %v", toolNames(tools))
	}
	if _, ok := tools["tail_stub"]; ok {
		t.Errorf("tail_stub should have retired in S4 but is still registered")
	}
}

// TestTailSinceDescriptionPresent guards the short description is
// non-empty and mentions the three key knobs (session/since_seq/limit).
func TestTailSinceDescriptionPresent(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	st := s.ListTools()["tail_since"]
	if st == nil {
		t.Fatal("tail_since not registered")
	}
	desc := strings.TrimSpace(st.Tool.Description)
	if desc == "" {
		t.Fatal("tail_since has empty Description")
	}
	for _, want := range []string{"session", "since_seq", "limit"} {
		if !strings.Contains(strings.ToLower(desc), want) {
			t.Errorf("description %q does not mention %q", desc, want)
		}
	}
}

// TestTailSinceInputSchemaAccepts exercises the canonical argument
// shapes per ADR-007 + ADR-008: session-only, with since_seq, with
// limit, all three. mcp-go v0.45.0 does not validate at dispatch, so
// "accepted" means the handler does not surface a Go error.
func TestTailSinceInputSchemaAccepts(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"next_since_seq":0}`))
	})
	c := newTestHubServer(t, h)
	cases := []struct {
		name string
		args map[string]any
	}{
		{"session only", map[string]any{"session": "s1"}},
		{"with since_seq", map[string]any{"session": "s1", "since_seq": float64(10)}},
		{"with limit", map[string]any{"session": "s1", "limit": float64(50)}},
		{"all three", map[string]any{"session": "s1", "since_seq": float64(10), "limit": float64(50)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callTailSince(t, c, tc.args)
			if res.IsError {
				t.Errorf("unexpected error result: %s", errorText(t, res))
			}
		})
	}
}

// TestTailSinceInputSchemaWrongTypesTolerated documents the mcp-go
// v0.45.0 behaviour: string-where-number is silently coerced by
// GetInt to its default (0). Tripwire test, fires automatically if
// mcp-go gains strict input validation.
func TestTailSinceInputSchemaWrongTypesTolerated(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrong-typed since_seq falls through GetInt's default;
		// the hub receives no `since_seq` query parameter.
		if r.URL.Query().Get("since_seq") != "" {
			t.Errorf("expected no since_seq query, got %q", r.URL.Query().Get("since_seq"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"next_since_seq":0}`))
	})
	c := newTestHubServer(t, h)
	res := callTailSince(t, c, map[string]any{
		"session":   "s1",
		"since_seq": "not-a-number",
	})
	if res.IsError {
		t.Errorf("expected success (type coerced to default), got: %s", errorText(t, res))
	}
}

// TestTailSinceMissingSessionFailsFast asserts that an absent or empty
// `session` argument fails inside the handler with a tool-result error
// — no hub round-trip is attempted.
func TestTailSinceMissingSessionFailsFast(t *testing.T) {
	t.Parallel()
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, `nope`, http.StatusInternalServerError)
	})
	c := newTestHubServer(t, h)

	for _, args := range []map[string]any{
		{},                         // session absent
		{"session": ""},            // empty session
		{"since_seq": float64(10)}, // session absent, other args present
	} {
		res := callTailSince(t, c, args)
		msg := errorText(t, res)
		if !strings.Contains(msg, "session required") {
			t.Errorf("args %v: error %q missing 'session required'", args, msg)
		}
	}
	if called {
		t.Error("hub was contacted despite missing session — expected fail-fast")
	}
}

// TestTailSinceHandlerCallsHub exercises the happy path: the handler
// invokes client.EventsSince against a httptest hub, the hub sees the
// bearer header and the canonical query string, and the result envelope
// carries the hub's response verbatim.
func TestTailSinceHandlerCallsHub(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath, gotQuery string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"events":[
				{"seq_id":11,"session_id":"s1","ts":1700000000,"source":"a","level":"info","msg":"first"},
				{"seq_id":17,"session_id":"s1","ts":1700000001,"source":"a","level":"warn","msg":"second"}
			],
			"next_since_seq":17
		}`))
	})
	c := newTestHubServer(t, h)

	res := callTailSince(t, c, map[string]any{
		"session":   "s1",
		"since_seq": float64(5),
		"limit":     float64(10),
	})
	body := decodeTailBody(t, res)

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization=%q", gotAuth)
	}
	if gotPath != "/events" {
		t.Errorf("path=%q, want /events", gotPath)
	}
	if gotQuery != "limit=10&session=s1&since_seq=5" {
		t.Errorf("query=%q, want canonical limit&session&since_seq", gotQuery)
	}
	if body.NextSinceSeq != 17 {
		t.Errorf("next_since_seq=%d, want 17", body.NextSinceSeq)
	}
	if len(body.Events) != 2 {
		t.Fatalf("len=%d, want 2", len(body.Events))
	}
	if body.Events[0].SeqID != 11 || body.Events[1].SeqID != 17 {
		t.Errorf("SeqIDs=[%d %d], want [11 17]", body.Events[0].SeqID, body.Events[1].SeqID)
	}
}

// TestTailSinceCursorAdvances drives two consecutive tool calls
// against a hub fake that paginates by since_seq — the canonical
// polling loop. First page returns seq 1..2 with next=2; second call
// passes since_seq=2 and gets seq 3..4 with next=4.
func TestTailSinceCursorAdvances(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		since := r.URL.Query().Get("since_seq")
		w.Header().Set("Content-Type", "application/json")
		switch since {
		case "", "0":
			_, _ = w.Write([]byte(`{
				"events":[
					{"seq_id":1,"session_id":"s1","ts":1,"source":"a","level":"info","msg":"e1"},
					{"seq_id":2,"session_id":"s1","ts":2,"source":"a","level":"info","msg":"e2"}
				],
				"next_since_seq":2
			}`))
		case "2":
			_, _ = w.Write([]byte(`{
				"events":[
					{"seq_id":3,"session_id":"s1","ts":3,"source":"a","level":"info","msg":"e3"},
					{"seq_id":4,"session_id":"s1","ts":4,"source":"a","level":"info","msg":"e4"}
				],
				"next_since_seq":4
			}`))
		default:
			_, _ = w.Write([]byte(`{"events":[],"next_since_seq":` + since + `}`))
		}
	})
	c := newTestHubServer(t, h)

	// Round 1.
	r1 := decodeTailBody(t, callTailSince(t, c, map[string]any{"session": "s1"}))
	if r1.NextSinceSeq != 2 || len(r1.Events) != 2 {
		t.Fatalf("round1: got next=%d len=%d, want next=2 len=2", r1.NextSinceSeq, len(r1.Events))
	}
	// Round 2.
	r2 := decodeTailBody(t, callTailSince(t, c, map[string]any{
		"session": "s1", "since_seq": float64(r1.NextSinceSeq),
	}))
	if r2.NextSinceSeq != 4 || len(r2.Events) != 2 {
		t.Fatalf("round2: got next=%d len=%d, want next=4 len=2", r2.NextSinceSeq, len(r2.Events))
	}
	if r2.Events[0].SeqID != 3 || r2.Events[1].SeqID != 4 {
		t.Errorf("round2 SeqIDs=[%d %d], want [3 4]", r2.Events[0].SeqID, r2.Events[1].SeqID)
	}
}

// TestTailSinceAuthFail asserts a 401 from the hub surfaces as a
// tool-result error carrying the unauthorized hint. Mirrors the
// sessions_list auth-fail test.
func TestTailSinceAuthFail(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c := newTestHubServer(t, h)
	res := callTailSince(t, c, map[string]any{"session": "s1"})
	msg := errorText(t, res)
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error %q missing 'unauthorized'", msg)
	}
}

// TestTailSinceEmptyResultEmitsArray asserts an empty hub response
// renders as `{"events":[], ...}` not `{"events":null, ...}` — JSON
// consumers (LLMs included) should never need to special-case null vs.
// empty. Also verifies next_since_seq echoes the caller's input
// ("stable on empty" — ADR-008).
func TestTailSinceEmptyResultEmitsArray(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"next_since_seq":42}`))
	})
	c := newTestHubServer(t, h)
	res := callTailSince(t, c, map[string]any{"session": "s1", "since_seq": float64(42)})
	if res.IsError {
		t.Fatalf("unexpected error: %s", errorText(t, res))
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T", res.Content[0])
	}
	if !strings.Contains(tc.Text, `"events":[]`) {
		t.Errorf("expected events:[] in body, got %q", tc.Text)
	}
	body := decodeTailBody(t, res)
	if body.NextSinceSeq != 42 {
		t.Errorf("next_since_seq=%d, want 42 (stable on empty)", body.NextSinceSeq)
	}
}
