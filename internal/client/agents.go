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
	"net/url"
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

// AgentIngestEventRef is one agent_event_refs row (Phase 2d S5-Tail,
// ADR-014 Accepted, Option B). All four fields are required; ref_type
// must be one of "observed" | "context" | "caused-by".
type AgentIngestEventRef struct {
	SpawnID string `json:"spawn_id"`
	EventID int64  `json:"event_id"`
	RefType string `json:"ref_type"`
	TS      int64  `json:"ts"`
}

// AgentIngestPayload is the request envelope for POST /agents/ingest.
// Source is required ("sdk-hook", "transcript", "mcp-push"); the
// nested records are each optional but at least one of
// spawn/tokens/verdicts/mailbox_edges/event_refs must be present (the
// server 400s on a fully empty payload).
type AgentIngestPayload struct {
	Source       string                   `json:"source"`
	Spawn        *AgentIngestSpawn        `json:"spawn,omitempty"`
	Tokens       []AgentIngestTokens      `json:"tokens,omitempty"`
	Verdicts     []AgentIngestVerdict     `json:"verdicts,omitempty"`
	MailboxEdges []AgentIngestMailboxEdge `json:"mailbox_edges,omitempty"`
	EventRefs    []AgentIngestEventRef    `json:"event_refs,omitempty"`
}

// AgentIngestCounts mirrors the server's
// internal/store.AgentInsertResult — per-table counts of rows that
// actually landed (vs. rows the INSERT OR IGNORE idempotency guard
// collapsed). A response of {0,0,0,0,0} is a fully-idempotent repeat.
//
// EventRefs added Phase 2d S5-Tail (ADR-014 Accepted, Option B).
type AgentIngestCounts struct {
	Spawns       int64 `json:"spawns"`
	Tokens       int64 `json:"tokens"`
	Verdicts     int64 `json:"verdicts"`
	MailboxEdges int64 `json:"mailbox_edges"`
	EventRefs    int64 `json:"event_refs"`
}

// AgentIngestResponse is the response envelope for POST /agents/ingest.
// Mirrors internal/agents.ingestResponse. The single `ingested` field
// keeps the response shape stable for forward-additions (future
// per-source breakdown, etc.).
type AgentIngestResponse struct {
	Ingested AgentIngestCounts `json:"ingested"`
}

// AgentSpawn is one agent_spawns row in the read-surface wire shape.
// Mirrors internal/agents.SpawnWire. StartedAt / EndedAt are unix-nano.
// Phase 2d S4 — pairs with AgentsSessions / AgentsTree.
type AgentSpawn struct {
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	Skill      string `json:"skill"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    *int64 `json:"ended_at,omitempty"`
	Project    string `json:"project"`
	SessionRef string `json:"session_ref,omitempty"`
}

// AgentSpawnsList is the response body for GET /agents/sessions.
// Total reflects the unfiltered count for the active filter, NOT just
// the rows returned in this page — callers can drive their own paging
// off it without an extra COUNT roundtrip.
type AgentSpawnsList struct {
	Spawns []AgentSpawn `json:"spawns"`
	Total  int64        `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

// AgentTreeNode is one spawn in the BFS-ordered tree response, plus a
// pre-computed depth so the consumer can indent without re-walking
// the parent chain. Mirrors internal/agents.TreeNodeWire.
type AgentTreeNode struct {
	AgentSpawn
	Depth int `json:"depth"`
}

// AgentTree is the response body for GET /agents/tree/{spawn_id}.
type AgentTree struct {
	Root  string          `json:"root"`
	Nodes []AgentTreeNode `json:"nodes"`
}

// AgentTokenRow is one agent_tokens row in the read wire shape. ts is
// unix-nano. Source is preserved so per-source breakdowns survive the
// HTTP boundary.
type AgentTokenRow struct {
	ID           int64  `json:"id"`
	SpawnID      string `json:"spawn_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheWrite   int64  `json:"cache_write"`
	TS           int64  `json:"ts"`
	Source       string `json:"source"`
}

// AgentTokenSourceSum is one per-source aggregate inside AgentTokenTotals.
type AgentTokenSourceSum struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	CacheRead    int64 `json:"cache_read"`
	CacheWrite   int64 `json:"cache_write"`
	RowCount     int64 `json:"row_count"`
}

// AgentTokenTotals is the aggregated counts shape on /agents/tokens.
// Total is the cross-source sum; BySource is keyed by canonical source
// ("sdk-hook" / "transcript" / "mcp-push"). Sources with zero rows are
// absent from the map.
type AgentTokenTotals struct {
	InputTokens  int64                          `json:"input_tokens"`
	OutputTokens int64                          `json:"output_tokens"`
	CacheRead    int64                          `json:"cache_read"`
	CacheWrite   int64                          `json:"cache_write"`
	BySource     map[string]AgentTokenSourceSum `json:"by_source"`
}

// AgentTokens is the response body for GET /agents/tokens?spawn_id=…
type AgentTokens struct {
	SpawnID string           `json:"spawn_id"`
	Tokens  []AgentTokenRow  `json:"tokens"`
	Totals  AgentTokenTotals `json:"totals"`
}

// AgentVerdictRow is one agent_verdicts row. ts is unix-nano;
// LerneffektMD is omitted when empty on the wire.
type AgentVerdictRow struct {
	ID           int64  `json:"id"`
	SpawnID      string `json:"spawn_id"`
	Verdict      string `json:"verdict"`
	LerneffektMD string `json:"lerneffekt_md,omitempty"`
	TS           int64  `json:"ts"`
}

// AgentVerdicts is the response body for GET /agents/verdicts?spawn_id=…
type AgentVerdicts struct {
	SpawnID  string            `json:"spawn_id"`
	Verdicts []AgentVerdictRow `json:"verdicts"`
}

// AgentEdgeRow is one agent_mailbox_edges row in the read wire shape.
// ts is unix-nano. Phase 2d S5.
type AgentEdgeRow struct {
	ID          int64  `json:"id"`
	FromSpawnID string `json:"from_spawn_id"`
	ToSpawnID   string `json:"to_spawn_id"`
	EdgeType    string `json:"edge_type"`
	TS          int64  `json:"ts"`
}

// AgentEdges is the response body for GET /agents/edges?spawn_id=…
// Two parallel slices — in-edges (rows pointing AT the spawn) and
// out-edges (rows pointing AWAY from the spawn). Both are always
// non-nil; empty when the spawn has no edges in that direction.
type AgentEdges struct {
	SpawnID string         `json:"spawn_id"`
	In      []AgentEdgeRow `json:"in"`
	Out     []AgentEdgeRow `json:"out"`
}

// AgentEventRefRow is one agent_event_refs row in the read wire shape.
// ts is unix-nano. Phase 2d S5-Tail (ADR-014 Accepted, Option B).
type AgentEventRefRow struct {
	ID      int64  `json:"id"`
	SpawnID string `json:"spawn_id"`
	EventID int64  `json:"event_id"`
	RefType string `json:"ref_type"`
	TS      int64  `json:"ts"`
}

// AgentEventRefs is the response body for GET /agents/event_refs?spawn_id=…
// Single slice — every row is anchored at the spawn_id; ordered ts ASC.
// EventRefs is always non-nil (empty when the spawn has no references).
//
// Phase 2d S5-Tail — ADR-014 Accepted (Option B, separate
// agent_event_refs table). Sibling to AgentsEdges; the two read
// surfaces stay independent.
type AgentEventRefs struct {
	SpawnID   string             `json:"spawn_id"`
	EventRefs []AgentEventRefRow `json:"event_refs"`
}

// AgentsSessions calls the hub's GET /agents/sessions endpoint.
// limit <= 0 means "use the hub default" (currently 50). offset < 0
// is treated as 0. project / sessionRef are optional exact-match
// filters; pass "" to omit.
func (c *Client) AgentsSessions(ctx context.Context, project, sessionRef string, limit, offset int) (AgentSpawnsList, error) {
	q := buildAgentsSessionsQuery(project, sessionRef, limit, offset)
	path := "/agents/sessions"
	if q != "" {
		path += "?" + q
	}
	var resp AgentSpawnsList
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     path,
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return AgentSpawnsList{}, err
	}
	if resp.Spawns == nil {
		resp.Spawns = []AgentSpawn{}
	}
	return resp, nil
}

// AgentsTree calls the hub's GET /agents/tree/{spawn_id} endpoint.
// Unknown spawn id surfaces as ErrNotFound (via the HTTPError 404 →
// sentinel mapping in doRequest).
func (c *Client) AgentsTree(ctx context.Context, spawnID string) (AgentTree, error) {
	if spawnID == "" {
		return AgentTree{}, errors.New("client: AgentsTree requires a non-empty spawn_id")
	}
	var resp AgentTree
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     "/agents/tree/" + url.PathEscape(spawnID),
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return AgentTree{}, err
	}
	if resp.Nodes == nil {
		resp.Nodes = []AgentTreeNode{}
	}
	return resp, nil
}

// AgentsTokens calls the hub's GET /agents/tokens?spawn_id=… endpoint.
func (c *Client) AgentsTokens(ctx context.Context, spawnID string) (AgentTokens, error) {
	if spawnID == "" {
		return AgentTokens{}, errors.New("client: AgentsTokens requires a non-empty spawn_id")
	}
	var resp AgentTokens
	q := url.Values{"spawn_id": []string{spawnID}}
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     "/agents/tokens?" + q.Encode(),
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return AgentTokens{}, err
	}
	if resp.Tokens == nil {
		resp.Tokens = []AgentTokenRow{}
	}
	if resp.Totals.BySource == nil {
		resp.Totals.BySource = map[string]AgentTokenSourceSum{}
	}
	return resp, nil
}

// AgentsEdges calls the hub's GET /agents/edges?spawn_id=… endpoint.
// Returns the in / out mailbox-edge slices for the spawn; both slices
// are always non-nil (empty when the spawn has no edges in a direction).
//
// Phase 2d S5 — mailbox-edge read surface.
func (c *Client) AgentsEdges(ctx context.Context, spawnID string) (AgentEdges, error) {
	if spawnID == "" {
		return AgentEdges{}, errors.New("client: AgentsEdges requires a non-empty spawn_id")
	}
	var resp AgentEdges
	q := url.Values{"spawn_id": []string{spawnID}}
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     "/agents/edges?" + q.Encode(),
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return AgentEdges{}, err
	}
	if resp.In == nil {
		resp.In = []AgentEdgeRow{}
	}
	if resp.Out == nil {
		resp.Out = []AgentEdgeRow{}
	}
	return resp, nil
}

// AgentsEventRefs calls the hub's GET /agents/event_refs?spawn_id=… endpoint.
// Returns every agent_event_refs row attached to the spawn (ordered ts ASC).
// EventRefs is always non-nil (empty when the spawn has no references).
//
// Phase 2d S5-Tail — ADR-014 Accepted (Option B, separate
// agent_event_refs table).
func (c *Client) AgentsEventRefs(ctx context.Context, spawnID string) (AgentEventRefs, error) {
	if spawnID == "" {
		return AgentEventRefs{}, errors.New("client: AgentsEventRefs requires a non-empty spawn_id")
	}
	var resp AgentEventRefs
	q := url.Values{"spawn_id": []string{spawnID}}
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     "/agents/event_refs?" + q.Encode(),
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return AgentEventRefs{}, err
	}
	if resp.EventRefs == nil {
		resp.EventRefs = []AgentEventRefRow{}
	}
	return resp, nil
}

// AgentsVerdicts calls the hub's GET /agents/verdicts?spawn_id=… endpoint.
func (c *Client) AgentsVerdicts(ctx context.Context, spawnID string) (AgentVerdicts, error) {
	if spawnID == "" {
		return AgentVerdicts{}, errors.New("client: AgentsVerdicts requires a non-empty spawn_id")
	}
	var resp AgentVerdicts
	q := url.Values{"spawn_id": []string{spawnID}}
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     "/agents/verdicts?" + q.Encode(),
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return AgentVerdicts{}, err
	}
	if resp.Verdicts == nil {
		resp.Verdicts = []AgentVerdictRow{}
	}
	return resp, nil
}

// buildAgentsSessionsQuery constructs the optional query string for
// AgentsSessions. Empty / zero / negative values are omitted entirely
// so the hub falls back to its own defaults.
func buildAgentsSessionsQuery(project, sessionRef string, limit, offset int) string {
	q := url.Values{}
	if project != "" {
		q.Set("project", project)
	}
	if sessionRef != "" {
		q.Set("session_ref", sessionRef)
	}
	if limit > 0 {
		q.Set("limit", itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", itoa(offset))
	}
	return q.Encode()
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
