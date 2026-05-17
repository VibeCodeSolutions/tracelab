package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

// TestAgentsIngest_HappyPath drives a full POST round-trip: the client
// marshals the envelope with source="mcp-push", the hub echoes
// per-table counts, the client surfaces them as AgentIngestCounts.
func TestAgentsIngest_HappyPath(t *testing.T) {
	var gotAuth, gotPath, gotMethod, gotCT string
	var gotBody []byte
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ingested":{"spawns":1,"tokens":1,"verdicts":0,"mailbox_edges":0}}`))
	})
	c, _ := newTestServer(t, h)

	endedAt := int64(1700000002_000000000)
	counts, err := c.AgentsIngest(context.Background(), AgentIngestPayload{
		Source: "mcp-push",
		Spawn: &AgentIngestSpawn{
			ID:        "spawn-mcp-test-aaaaaaaaaaaaaaaa",
			Skill:     "ballard",
			StartedAt: 1700000001_000000000,
			EndedAt:   &endedAt,
			Project:   "tracelab",
		},
		Tokens: []AgentIngestTokens{{
			SpawnID:      "spawn-mcp-test-aaaaaaaaaaaaaaaa",
			InputTokens:  100,
			OutputTokens: 200,
			TS:           1700000002_000000000,
		}},
	})
	if err != nil {
		t.Fatalf("AgentsIngest: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method=%q, want POST", gotMethod)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization=%q", gotAuth)
	}
	if gotPath != "/agents/ingest" {
		t.Errorf("path=%q, want /agents/ingest", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type=%q, want application/json", gotCT)
	}
	// Spot-check the body has the canonical source discriminator.
	var sent AgentIngestPayload
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("body not valid JSON: %v (%s)", err, string(gotBody))
	}
	if sent.Source != "mcp-push" {
		t.Errorf("body source=%q, want mcp-push", sent.Source)
	}
	if sent.Spawn == nil || sent.Spawn.Project != "tracelab" {
		t.Errorf("body spawn missing/wrong: %+v", sent.Spawn)
	}
	if counts.Spawns != 1 || counts.Tokens != 1 {
		t.Errorf("counts=%+v, want spawns=1 tokens=1", counts)
	}
}

// TestAgentsIngest_EmptySource asserts the client-side fast-fail
// — no HTTP round-trip when the source discriminator is empty.
func TestAgentsIngest_EmptySource(t *testing.T) {
	var called bool
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	_, err := c.AgentsIngest(context.Background(), AgentIngestPayload{})
	if err == nil {
		t.Fatal("expected error for empty source")
	}
	if called {
		t.Error("hub was contacted despite empty source — expected fail-fast")
	}
}

// TestAgentsIngest_Idempotency: a {0,0,0,0} counts response is the
// canonical idempotent-repeat signal. Client surfaces it verbatim.
func TestAgentsIngest_Idempotency(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ingested":{"spawns":0,"tokens":0,"verdicts":0,"mailbox_edges":0}}`))
	})
	c, _ := newTestServer(t, h)
	counts, err := c.AgentsIngest(context.Background(), AgentIngestPayload{
		Source: "mcp-push",
		Spawn: &AgentIngestSpawn{
			ID:        "spawn-mcp-test-aaaaaaaaaaaaaaaa",
			Skill:     "ballard",
			StartedAt: 1700000001_000000000,
			Project:   "tracelab",
		},
	})
	if err != nil {
		t.Fatalf("AgentsIngest: %v", err)
	}
	if counts != (AgentIngestCounts{}) {
		t.Errorf("counts=%+v, want zero-value (idempotent repeat)", counts)
	}
}

// TestAgentsIngest_Unauthorized: 401 maps to ErrUnauthorized sentinel.
func TestAgentsIngest_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	_, err := c.AgentsIngest(context.Background(), AgentIngestPayload{
		Source: "mcp-push",
		Spawn: &AgentIngestSpawn{
			ID: "x", Skill: "ballard", StartedAt: 1, Project: "tracelab",
		},
	})
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err=%v, want ErrUnauthorized", err)
	}
}

// TestAgentsIngest_BadRequest: 400 (hub rejects e.g. empty payload or
// unknown source) surfaces as *HTTPError without sentinel wrapping.
func TestAgentsIngest_BadRequest(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"payload is empty"}`, http.StatusBadRequest)
	})
	c, _ := newTestServer(t, h)
	_, err := c.AgentsIngest(context.Background(), AgentIngestPayload{
		Source: "mcp-push",
		Spawn: &AgentIngestSpawn{
			ID: "x", Skill: "ballard", StartedAt: 1, Project: "tracelab",
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.Status != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", httpErr.Status)
	}
}

// TestAgentsIngest_ServerError: 500 maps to ErrServerError sentinel.
func TestAgentsIngest_ServerError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `boom`, http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	_, err := c.AgentsIngest(context.Background(), AgentIngestPayload{
		Source: "mcp-push",
		Spawn: &AgentIngestSpawn{
			ID: "x", Skill: "ballard", StartedAt: 1, Project: "tracelab",
		},
	})
	if !errors.Is(err, ErrServerError) {
		t.Errorf("err=%v, want ErrServerError", err)
	}
}

// ─── Phase 2d S4 — Read-method tests ───────────────────────────────────

// TestAgentsSessions_HappyPath drives a full GET round-trip: the client
// requests /agents/sessions with paging knobs, the hub echoes a typed
// envelope, the client returns AgentSpawnsList with non-nil spawns.
func TestAgentsSessions_HappyPath(t *testing.T) {
	var gotPath, gotMethod string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"spawns":[
				{"id":"a1","skill":"ballard","started_at":1700000000,"project":"tracelab"}
			],
			"total":1,"limit":50,"offset":0
		}`))
	})
	c, _ := newTestServer(t, h)
	resp, err := c.AgentsSessions(context.Background(), "tracelab", "", 50, 0)
	if err != nil {
		t.Fatalf("AgentsSessions: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method=%q, want GET", gotMethod)
	}
	if !contains(gotPath, "project=tracelab") {
		t.Errorf("query missing project filter: %q", gotPath)
	}
	if !contains(gotPath, "limit=50") {
		t.Errorf("query missing limit: %q", gotPath)
	}
	if resp.Total != 1 || len(resp.Spawns) != 1 {
		t.Errorf("resp=%+v, want total=1 spawns=1", resp)
	}
	if resp.Spawns[0].Skill != "ballard" {
		t.Errorf("first spawn skill=%q", resp.Spawns[0].Skill)
	}
}

// TestAgentsSessions_EmptyResponseNonNil — a hub returning `"spawns":null`
// must surface as an empty slice (NOT nil), mirroring the CrashesList
// posture.
func TestAgentsSessions_EmptyResponseNonNil(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spawns":null,"total":0,"limit":50,"offset":0}`))
	})
	c, _ := newTestServer(t, h)
	resp, err := c.AgentsSessions(context.Background(), "", "", 0, 0)
	if err != nil {
		t.Fatalf("AgentsSessions: %v", err)
	}
	if resp.Spawns == nil {
		t.Error("Spawns must be non-nil even on empty hub response")
	}
}

func TestAgentsTree_HappyPath(t *testing.T) {
	var gotPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"root":"a1","nodes":[
			{"id":"a1","skill":"belanna","started_at":1700000000,"project":"tracelab","depth":0},
			{"id":"a2","parent_id":"a1","skill":"ballard","started_at":1700000001,"project":"tracelab","depth":1}
		]}`))
	})
	c, _ := newTestServer(t, h)
	tree, err := c.AgentsTree(context.Background(), "a1")
	if err != nil {
		t.Fatalf("AgentsTree: %v", err)
	}
	if gotPath != "/agents/tree/a1" {
		t.Errorf("path=%q, want /agents/tree/a1", gotPath)
	}
	if tree.Root != "a1" {
		t.Errorf("root=%q, want a1", tree.Root)
	}
	if len(tree.Nodes) != 2 {
		t.Fatalf("nodes len=%d, want 2", len(tree.Nodes))
	}
	if tree.Nodes[1].Depth != 1 {
		t.Errorf("child depth=%d, want 1", tree.Nodes[1].Depth)
	}
}

func TestAgentsTree_EmptyID(t *testing.T) {
	h := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be called on empty id")
	})
	c, _ := newTestServer(t, h)
	if _, err := c.AgentsTree(context.Background(), ""); err == nil {
		t.Error("expected error for empty spawn_id")
	}
}

func TestAgentsTokens_AggregatesAndBreakdown(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hub-side path should encode spawn_id correctly.
		if !contains(r.URL.RawQuery, "spawn_id=spawn1") {
			t.Errorf("query missing spawn_id: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"spawn_id":"spawn1",
			"tokens":[
				{"id":1,"spawn_id":"spawn1","input_tokens":100,"output_tokens":50,"cache_read":0,"cache_write":0,"ts":1700000000,"source":"sdk-hook"},
				{"id":2,"spawn_id":"spawn1","input_tokens":200,"output_tokens":100,"cache_read":0,"cache_write":0,"ts":1700000001,"source":"transcript"}
			],
			"totals":{
				"input_tokens":300,"output_tokens":150,"cache_read":0,"cache_write":0,
				"by_source":{
					"sdk-hook":{"input_tokens":100,"output_tokens":50,"cache_read":0,"cache_write":0,"row_count":1},
					"transcript":{"input_tokens":200,"output_tokens":100,"cache_read":0,"cache_write":0,"row_count":1}
				}
			}
		}`))
	})
	c, _ := newTestServer(t, h)
	resp, err := c.AgentsTokens(context.Background(), "spawn1")
	if err != nil {
		t.Fatalf("AgentsTokens: %v", err)
	}
	if resp.Totals.InputTokens != 300 || resp.Totals.OutputTokens != 150 {
		t.Errorf("totals=%+v", resp.Totals)
	}
	if len(resp.Totals.BySource) != 2 {
		t.Errorf("by_source keys=%d, want 2", len(resp.Totals.BySource))
	}
	if resp.Totals.BySource["sdk-hook"].InputTokens != 100 {
		t.Errorf("sdk-hook breakdown wrong: %+v", resp.Totals.BySource["sdk-hook"])
	}
}

func TestAgentsVerdicts_HappyPath(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spawn_id":"x","verdicts":[
			{"id":1,"spawn_id":"x","verdict":"freigabe","ts":1700000000}
		]}`))
	})
	c, _ := newTestServer(t, h)
	resp, err := c.AgentsVerdicts(context.Background(), "x")
	if err != nil {
		t.Fatalf("AgentsVerdicts: %v", err)
	}
	if len(resp.Verdicts) != 1 || resp.Verdicts[0].Verdict != "freigabe" {
		t.Errorf("verdicts=%+v", resp.Verdicts)
	}
}

func TestAgentsReadEndpoints_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `unauthorized`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	if _, err := c.AgentsSessions(context.Background(), "", "", 0, 0); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("AgentsSessions err=%v, want ErrUnauthorized", err)
	}
	if _, err := c.AgentsTokens(context.Background(), "x"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("AgentsTokens err=%v", err)
	}
}

// contains is a tiny test-only helper to avoid pulling in strings just
// for substring checks scattered through the response-shape assertions.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
