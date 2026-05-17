// Phase 2d S3 — agents.go owns the write against the hub's
// POST /agents/ingest endpoint (ADR-013, Phase 2d S1). It is the
// shared push surface for the MCP agent_event tool (Phase 2d S3) and
// any future caller that needs to feed the multi-ingest pipeline from
// outside the hub process.
//
// Semantics in one paragraph:
//
//   - "Push an agent-lifecycle envelope (spawn/tokens/verdicts/edges)
//     into the hub's /agents/ingest endpoint, idempotency enforced
//     server-side via INSERT OR IGNORE on UNIQUE-tuple indexes
//     (ADR-013)."
//   - The source discriminator is REQUIRED in every envelope — the
//     server pre-validates and 400s on an unknown source.
//   - Time fields are unix-nano (NOT unix-ms) — same convention as
//     sessions/events/crashes. Pre-Hardcoding-Verifikation #034
//     verified this against internal/agents/payload.go +
//     internal/store/agents.go.
//   - Bearer auth and HTTPError sentinels (ErrUnauthorized /
//     ErrServerError) are the same as every other authenticated client
//     method; no /agents-specific error shape.
//
// Wire-shape mirrors internal/agents.IngestPayload. We keep a
// separate client-side type (rather than importing the server type)
// for the same reason the rest of internal/client does: keep the
// public client surface free of server-side internals.
package client

import (
	"context"
	"errors"
	"net/http"
)

// AgentIngestSpawn is one agent_spawns row in the wire envelope.
// Mirrors internal/agents.SpawnPayload. ID is writer-supplied
// (26-char ULID-shaped string). StartedAt/EndedAt are unix-nano.
type AgentIngestSpawn struct {
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	Skill      string `json:"skill"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    *int64 `json:"ended_at,omitempty"`
	Project    string `json:"project"`
	SessionRef string `json:"session_ref,omitempty"`
}

// AgentIngestTokens is one agent_tokens delta. Source is taken from
// the envelope's IngestPayload.Source on the server side.
type AgentIngestTokens struct {
	SpawnID      string `json:"spawn_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheWrite   int64  `json:"cache_write"`
	TS           int64  `json:"ts"`
}

// AgentIngestVerdict is one agent_verdicts row.
type AgentIngestVerdict struct {
	SpawnID      string `json:"spawn_id"`
	Verdict      string `json:"verdict"`
	LerneffektMD string `json:"lerneffekt_md,omitempty"`
	TS           int64  `json:"ts"`
}

// AgentIngestMailboxEdge is one agent_mailbox_edges row.
type AgentIngestMailboxEdge struct {
	FromSpawnID string `json:"from_spawn_id"`
	ToSpawnID   string `json:"to_spawn_id"`
	EdgeType    string `json:"edge_type"`
	TS          int64  `json:"ts"`
}

// AgentIngestPayload is the request envelope for POST /agents/ingest.
// Source is required ("sdk-hook", "transcript", "mcp-push"); the
// nested records are each optional but at least one of
// spawn/tokens/verdicts/mailbox_edges must be present (the server
// 400s on a fully empty payload).
type AgentIngestPayload struct {
	Source       string                   `json:"source"`
	Spawn        *AgentIngestSpawn        `json:"spawn,omitempty"`
	Tokens       []AgentIngestTokens      `json:"tokens,omitempty"`
	Verdicts     []AgentIngestVerdict     `json:"verdicts,omitempty"`
	MailboxEdges []AgentIngestMailboxEdge `json:"mailbox_edges,omitempty"`
}

// AgentIngestCounts mirrors the server's
// internal/store.AgentInsertResult — per-table counts of rows that
// actually landed (vs. rows the INSERT OR IGNORE idempotency guard
// collapsed). A response of {0,0,0,0} is a fully-idempotent repeat.
type AgentIngestCounts struct {
	Spawns       int64 `json:"spawns"`
	Tokens       int64 `json:"tokens"`
	Verdicts     int64 `json:"verdicts"`
	MailboxEdges int64 `json:"mailbox_edges"`
}

// AgentIngestResponse is the response envelope for POST /agents/ingest.
// Mirrors internal/agents.ingestResponse. The single `ingested` field
// keeps the response shape stable for forward-additions (future
// per-source breakdown, etc.).
type AgentIngestResponse struct {
	Ingested AgentIngestCounts `json:"ingested"`
}

// AgentsIngest calls the hub's POST /agents/ingest endpoint and
// returns the per-table forensic counts.
//
//   - source must be non-empty and one of the three canonical
//     discriminators ("sdk-hook" / "transcript" / "mcp-push"); the
//     hub validates and 400s on anything else, but we fail-fast on
//     the empty case here to spare an HTTP round-trip.
//   - The payload must carry at least one of spawn/tokens/verdicts/
//     mailbox_edges (the hub 400s on a fully empty envelope).
//
// On a 200 the returned counts let the caller distinguish "first
// write" (counts > 0) from "idempotent repeat" (counts = 0).
func (c *Client) AgentsIngest(ctx context.Context, payload AgentIngestPayload) (AgentIngestCounts, error) {
	if payload.Source == "" {
		return AgentIngestCounts{}, errors.New("client: AgentsIngest requires a non-empty source")
	}

	var resp AgentIngestResponse
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodPost,
		path:     "/agents/ingest",
		body:     payload,
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return AgentIngestCounts{}, err
	}
	return resp.Ingested, nil
}
