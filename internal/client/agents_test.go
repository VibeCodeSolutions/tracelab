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
