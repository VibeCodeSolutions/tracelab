package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// callAgentEvent drives the agent_event handler closure with a
// constructed CallToolRequest. Mirrors callCrashesList / callSessionsList
// — we test the handler directly so failure messages name the tool
// semantics, not the transport.
func callAgentEvent(t *testing.T, c *client.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := newAgentEventTool(c)
	req := mcp.CallToolRequest{}
	req.Params.Name = agentEventToolName
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

// decodeAgentEventBody extracts the JSON-encoded {"ingested":{...}}
// envelope from a tool result's first TextContent.
func decodeAgentEventBody(t *testing.T, res *mcp.CallToolResult) agentEventResult {
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
	var out agentEventResult
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("decode body %q: %v", tc.Text, err)
	}
	return out
}

// TestAgentEventToolRegistered confirms agent_event is registered as
// the seventh real tool. Mirrors crashes_list / sessions_list
// registration smoke tests.
func TestAgentEventToolRegistered(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	tools := s.ListTools()
	if _, ok := tools["agent_event"]; !ok {
		t.Errorf("agent_event missing from registry; got %v", toolNames(tools))
	}
}

// TestAgentEventDescriptionMentionsTimeUnit guards the load-bearing
// hint that every time field is unix-nano. The briefing-text said
// unix-ms — the Pre-Hardcoding-Verifikation step caught this; the
// description carries the correction so an LLM caller cannot silently
// regress to ms.
func TestAgentEventDescriptionMentionsTimeUnit(t *testing.T) {
	t.Parallel()
	c := newTestHubServer(t, http.NotFoundHandler())
	s := buildServer(c)
	st := s.ListTools()["agent_event"]
	if st == nil {
		t.Fatal("agent_event not registered")
	}
	desc := strings.ToLower(strings.TrimSpace(st.Tool.Description))
	if desc == "" {
		t.Fatal("agent_event has empty Description")
	}
	if !strings.Contains(desc, "unix-nano") {
		t.Errorf("description %q does not mention unix-nano (load-bearing time-unit hint)", desc)
	}
	// Also assert mcp-push is named — the tool's identity IS the
	// source discriminator (ADR-013).
	if !strings.Contains(desc, "mcp-push") {
		t.Errorf("description %q does not mention source=mcp-push", desc)
	}
}

// TestAgentEventToolSmoke drives the happy path end-to-end against a
// httptest hub: the handler builds an envelope with hardcoded
// source="mcp-push", forwards it to /agents/ingest, and surfaces the
// per-table counts verbatim.
func TestAgentEventToolSmoke(t *testing.T) {
	t.Parallel()
	var gotPath, gotMethod, gotAuth string
	var gotBody []byte
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ingested":{"spawns":1,"tokens":1,"verdicts":1,"mailbox_edges":0}}`))
	})
	c := newTestHubServer(t, h)

	res := callAgentEvent(t, c, map[string]any{
		"spawn": map[string]any{
			"id":         "agent-event-smoke-aaaaaaaaaa",
			"skill":      "ballard",
			"project":    "tracelab",
			"started_at": float64(1700000001_000000000),
			"ended_at":   float64(1700000002_000000000),
		},
		"tokens": []any{
			map[string]any{
				"spawn_id":      "agent-event-smoke-aaaaaaaaaa",
				"input_tokens":  float64(123),
				"output_tokens": float64(456),
				"ts":            float64(1700000002_000000000),
			},
		},
		"verdicts": []any{
			map[string]any{
				"spawn_id":      "agent-event-smoke-aaaaaaaaaa",
				"verdict":       "freigabe",
				"lerneffekt_md": "test-note",
				"ts":            float64(1700000002_000000000),
			},
		},
	})
	body := decodeAgentEventBody(t, res)

	if gotMethod != http.MethodPost {
		t.Errorf("hub method=%q, want POST", gotMethod)
	}
	if gotPath != "/agents/ingest" {
		t.Errorf("hub path=%q, want /agents/ingest", gotPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization=%q", gotAuth)
	}
	// Verify the source discriminator on the wire is hardcoded
	// mcp-push (Pre-Hardcoding-Verifikation: the tool's identity
	// IS the source — never trust caller-supplied values).
	var sent client.AgentIngestPayload
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("hub got non-JSON body: %v (%s)", err, string(gotBody))
	}
	if sent.Source != "mcp-push" {
		t.Errorf("wire source=%q, want mcp-push", sent.Source)
	}
	if sent.Spawn == nil || sent.Spawn.ID != "agent-event-smoke-aaaaaaaaaa" {
		t.Errorf("wire spawn missing/wrong: %+v", sent.Spawn)
	}
	if sent.Spawn != nil && sent.Spawn.StartedAt != 1700000001_000000000 {
		t.Errorf("wire started_at=%d, want 1700000001000000000 (unix-nano)", sent.Spawn.StartedAt)
	}
	if len(sent.Tokens) != 1 || sent.Tokens[0].InputTokens != 123 {
		t.Errorf("wire tokens wrong: %+v", sent.Tokens)
	}
	if body.Ingested.Spawns != 1 || body.Ingested.Tokens != 1 || body.Ingested.Verdicts != 1 {
		t.Errorf("counts=%+v, want spawns/tokens/verdicts=1", body.Ingested)
	}
}

// TestAgentEventToolIdempotency: the hub responds with {0,0,0,0}
// (canonical idempotent-repeat signal); the tool propagates it
// verbatim so the caller can audit "this push was a no-op".
func TestAgentEventToolIdempotency(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ingested":{"spawns":0,"tokens":0,"verdicts":0,"mailbox_edges":0}}`))
	})
	c := newTestHubServer(t, h)

	res := callAgentEvent(t, c, map[string]any{
		"spawn": map[string]any{
			"id":         "agent-event-idem-aaaaaaaaaaaa",
			"skill":      "ballard",
			"project":    "tracelab",
			"started_at": float64(1700000010_000000000),
		},
	})
	body := decodeAgentEventBody(t, res)
	if body.Ingested != (client.AgentIngestCounts{}) {
		t.Errorf("counts=%+v, want zero-value (idempotent repeat)", body.Ingested)
	}
}

// TestAgentEventToolEmptyPayloadFailsFast asserts an envelope with no
// spawn / tokens / verdicts / mailbox_edges fails at the tool layer
// with NO hub round-trip. Mirrors the server's validate() so a caller
// gets the same actionable message regardless of where the check
// fires.
func TestAgentEventToolEmptyPayloadFailsFast(t *testing.T) {
	t.Parallel()
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, `should not be reached`, http.StatusInternalServerError)
	})
	c := newTestHubServer(t, h)

	res := callAgentEvent(t, c, map[string]any{})
	msg := errorText(t, res)
	if !strings.Contains(msg, "payload is empty") {
		t.Errorf("error %q missing 'payload is empty'", msg)
	}
	if called {
		t.Error("hub was contacted despite empty payload — expected fail-fast")
	}
}

// TestAgentEventToolBadAuth: a 401 from the hub propagates as a
// translated tool-result error carrying the unauthorized hint. Mirrors
// crashes_list / sessions_list auth-fail tests.
func TestAgentEventToolBadAuth(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c := newTestHubServer(t, h)
	res := callAgentEvent(t, c, map[string]any{
		"spawn": map[string]any{
			"id":         "agent-event-auth-aaaaaaaaaaaa",
			"skill":      "ballard",
			"project":    "tracelab",
			"started_at": float64(1700000020_000000000),
		},
	})
	msg := errorText(t, res)
	if !strings.Contains(msg, "unauthorized") {
		t.Errorf("error %q missing 'unauthorized'", msg)
	}
}

// TestAgentEventToolSchemaValidationCoercion documents the mcp-go
// v0.45.0 behaviour: nested wrong-typed fields surface via the
// BindArguments(JSON-unmarshal) path. A non-numeric started_at fails
// the JSON-unmarshal into int64 and the tool reports it as a
// tool-result error WITHOUT a hub round-trip.
func TestAgentEventToolSchemaValidationCoercion(t *testing.T) {
	t.Parallel()
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		http.Error(w, `should not be reached`, http.StatusInternalServerError)
	})
	c := newTestHubServer(t, h)
	res := callAgentEvent(t, c, map[string]any{
		"spawn": map[string]any{
			"id":         "agent-event-bad-aaaaaaaaaaaa",
			"skill":      "ballard",
			"project":    "tracelab",
			"started_at": "not-a-number",
		},
	})
	if !res.IsError {
		t.Fatal("expected IsError=true for wrong-typed started_at")
	}
	msg := errorText(t, res)
	if !strings.Contains(msg, "invalid arguments") {
		t.Errorf("error %q missing 'invalid arguments'", msg)
	}
	if called {
		t.Error("hub was contacted despite bind failure — expected fail-fast")
	}
}

// TestAgentEventToolForensicCountsForwarded: non-trivial mixed counts
// (some tables non-zero, others zero) survive the round-trip with
// per-field fidelity. Guards against accidental field-swap in the
// JSON re-encode path.
func TestAgentEventToolForensicCountsForwarded(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ingested":{"spawns":0,"tokens":3,"verdicts":1,"mailbox_edges":2}}`))
	})
	c := newTestHubServer(t, h)
	res := callAgentEvent(t, c, map[string]any{
		"spawn": map[string]any{
			"id":         "agent-event-fc-aaaaaaaaaaaaa",
			"skill":      "ballard",
			"project":    "tracelab",
			"started_at": float64(1700000030_000000000),
		},
	})
	body := decodeAgentEventBody(t, res)
	got := body.Ingested
	want := client.AgentIngestCounts{Spawns: 0, Tokens: 3, Verdicts: 1, MailboxEdges: 2}
	if got != want {
		t.Errorf("counts=%+v, want %+v", got, want)
	}
}
