// agent_event — seventh real MCP tool (Phase 2d S3).
//
// Per ADR-013 (Multi-Ingest agent stack, Phase 2d S0) and the
// `/agents/ingest` endpoint shape locked in Phase 2d S1:
//
//   - Tool name:   agent_event (snake_case <noun>_<noun>, no
//     `tracelab_` prefix — MCP ecosystem convention, see ADR-007).
//   - Purpose:     third and last of the three ingest sources for
//     the Phase-2d agent stack. SDK-hooks push lifecycle events
//     from the Claude-Code hook surface (S1); transcript-tail
//     scrapes token-usage from JSONL (S2); agent_event lets a
//     running worker (or any MCP client) push its own envelope
//     directly. The handler hardcodes source="mcp-push" before
//     forwarding to the hub.
//   - Input:       a flat object that mirrors internal/agents.
//     IngestPayload — `spawn` (object, required), `tokens` (array),
//     `verdicts` (array), `mailbox_edges` (array). Source is
//     deliberately NOT exposed as a tool parameter: the tool's
//     identity IS the source. Time fields are unix-nano (NOT
//     unix-ms) — verified against internal/agents/payload.go.
//   - Output:      JSON-encoded TextContent carrying the hub's
//     per-table forensic counts verbatim:
//     `{"ingested":{"spawns":N,"tokens":N,"verdicts":N,
//     "mailbox_edges":N}}`. A {0,0,0,0} response is the canonical
//     idempotent-repeat signal (the writer's second push was a
//     no-op on the UNIQUE-tuple indexes — ADR-013 §Consequences).
//   - Hub call:    POST /agents/ingest via client.AgentsIngest.
//     Bearer is attached by the shared *client.Client constructed
//     in newServer (same plumbing as the six S3-S6 tools).
//
// Pre-Hardcoding-Verifikation (Phase 2d S3, AUFTRAG #034 — third
// application of the pattern):
//
//   - `started_at` and all other time fields are unix-NANO (the
//     briefing-text said unix-ms; payload.go's validate() rejects
//     anything that fails the "unix-nano > 0" gate, and store.go
//     calls UnixNano() unconditionally). The tool description spells
//     out "unix-nano" so an LLM caller does not silently send ms.
//   - `tokens` and `verdicts` are TOP-LEVEL arrays in IngestPayload,
//     NOT embedded sub-structs in the spawn record. The briefing's
//     "verdict (struct ... + ts)" wording would have produced a
//     400 from the server's validate(). The tool schema mirrors the
//     server-side shape exactly.
//   - mcp-go v0.45.0 (pinned in go.mod from P2b-S1) is unchanged.
//     We use BindArguments to unmarshal the request into the
//     typed AgentEventInput — the same path mcp-go's own
//     tools_test.go exercises (BindArgumentsWithRawJSON).
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// agentEventToolName is the canonical tool name; pinned in one place
// so tests, registration, and any future error message all reference
// the same literal.
const agentEventToolName = "agent_event"

// agentEventDescription is the short description surfaced via
// tools/list. One sentence per ADR-007; the time-unit hint is
// load-bearing — every time field is unix-NANO (not unix-ms).
const agentEventDescription = "Push an agent-lifecycle event into the tracelab hub (multi-ingest source mcp-push). Accepts a spawn record plus optional tokens/verdicts/mailbox_edges arrays. All time fields are unix-nano. Idempotent: a repeat-call returns zero counts."

// agentEventSource is the discriminator the handler stamps onto every
// envelope. Hardcoded — the tool's identity IS the source (an
// untrusted caller cannot impersonate the SDK-hook or transcript-tail
// paths by smuggling a different value).
const agentEventSource = "mcp-push"

// AgentEventInput is the typed shape the tool's BindArguments call
// unmarshals into. Mirrors internal/agents.IngestPayload minus the
// `source` field. JSON tags match the server-side wire shape so the
// handler can re-emit the same envelope to client.AgentsIngest
// without per-field copying.
type AgentEventInput struct {
	Spawn        *agentEventSpawn        `json:"spawn,omitempty"`
	Tokens       []agentEventTokens      `json:"tokens,omitempty"`
	Verdicts     []agentEventVerdict     `json:"verdicts,omitempty"`
	MailboxEdges []agentEventMailboxEdge `json:"mailbox_edges,omitempty"`
}

type agentEventSpawn struct {
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	Skill      string `json:"skill"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    *int64 `json:"ended_at,omitempty"`
	Project    string `json:"project"`
	SessionRef string `json:"session_ref,omitempty"`
}

type agentEventTokens struct {
	SpawnID      string `json:"spawn_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheWrite   int64  `json:"cache_write"`
	TS           int64  `json:"ts"`
}

type agentEventVerdict struct {
	SpawnID      string `json:"spawn_id"`
	Verdict      string `json:"verdict"`
	LerneffektMD string `json:"lerneffekt_md,omitempty"`
	TS           int64  `json:"ts"`
}

type agentEventMailboxEdge struct {
	FromSpawnID string `json:"from_spawn_id"`
	ToSpawnID   string `json:"to_spawn_id"`
	EdgeType    string `json:"edge_type"`
	TS          int64  `json:"ts"`
}

// agentEventResult is the public output envelope. JSON-encoded into
// a single TextContent per ADR-007. The field name `ingested`
// mirrors the hub's response so an operator copying the tool output
// into a hub-debug log sees the same shape.
type agentEventResult struct {
	Ingested client.AgentIngestCounts `json:"ingested"`
}

// newAgentEventTool builds the ServerTool registered into the MCP
// server. The closure captures c so each invocation reuses the same
// hub client (bearer-bound at server-start).
//
// The schema describes the top-level optionals (spawn / tokens /
// verdicts / mailbox_edges) plus their nested shapes via WithObject
// / WithArray. mcp-go v0.45.0 does not enforce nested-schema
// validation at dispatch (verified against tools_test.go), so the
// real safety net is the server-side validate() in
// internal/agents/payload.go — the tool schema is documentation
// for the LLM caller, not a wire-level firewall.
func newAgentEventTool(c *client.Client) server.ServerTool {
	tool := mcp.NewTool(
		agentEventToolName,
		mcp.WithDescription(agentEventDescription),
		mcp.WithObject("spawn",
			mcp.Description("Agent spawn lifecycle row. Required if no other section is present. Time fields are unix-nano."),
		),
		mcp.WithArray("tokens",
			mcp.Description("Token-usage deltas, one entry per accounting point. Each entry needs spawn_id and ts (unix-nano)."),
		),
		mcp.WithArray("verdicts",
			mcp.Description("QS verdicts attached to a spawn. Verdict must be one of freigabe|auflagen|rueckgabe|eskalation|none."),
		),
		mcp.WithArray("mailbox_edges",
			mcp.Description("Mailbox relations between spawns. Edge type must be one of spawn|return|escalate|delegate."),
		),
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: agentEventHandler(c),
	}
}

// agentEventHandler is the typed handler closure. Split from
// newAgentEventTool so unit tests can invoke it without driving the
// MCP dispatch path.
//
// Flow:
//
//  1. BindArguments unmarshals the JSON request into AgentEventInput.
//     A bind failure (e.g. wrong-typed nested field) surfaces as a
//     tool-result error before any hub round-trip — same fail-fast
//     pattern as crashes_list / sessions_list.
//  2. Empty-envelope guard mirrors the server's validate() so the
//     caller gets a useful message at the tool layer rather than a
//     400 from the hub. (The server still rejects on its own — this
//     is defence in depth, not validation duplication.)
//  3. Build the typed client.AgentIngestPayload with hardcoded
//     source="mcp-push" (a caller cannot impersonate another
//     ingest path).
//  4. Forward to client.AgentsIngest, which handles bearer auth,
//     timeout, and sentinel-mapping (401 → ErrUnauthorized, 5xx →
//     ErrServerError, 4xx other → *HTTPError).
//  5. Re-encode the per-table counts as TextContent JSON. {0,0,0,0}
//     is the idempotent-repeat signal and propagates verbatim.
func agentEventHandler(c *client.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var in AgentEventInput
		if err := req.BindArguments(&in); err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("invalid arguments: %v", err),
			), nil
		}

		if in.Spawn == nil && len(in.Tokens) == 0 &&
			len(in.Verdicts) == 0 && len(in.MailboxEdges) == 0 {
			return mcp.NewToolResultError(
				"payload is empty (need at least one of spawn|tokens|verdicts|mailbox_edges)",
			), nil
		}

		payload := client.AgentIngestPayload{Source: agentEventSource}
		if in.Spawn != nil {
			payload.Spawn = &client.AgentIngestSpawn{
				ID:         in.Spawn.ID,
				ParentID:   in.Spawn.ParentID,
				Skill:      in.Spawn.Skill,
				StartedAt:  in.Spawn.StartedAt,
				EndedAt:    in.Spawn.EndedAt,
				Project:    in.Spawn.Project,
				SessionRef: in.Spawn.SessionRef,
			}
		}
		if len(in.Tokens) > 0 {
			payload.Tokens = make([]client.AgentIngestTokens, len(in.Tokens))
			for i, t := range in.Tokens {
				payload.Tokens[i] = client.AgentIngestTokens{
					SpawnID:      t.SpawnID,
					InputTokens:  t.InputTokens,
					OutputTokens: t.OutputTokens,
					CacheRead:    t.CacheRead,
					CacheWrite:   t.CacheWrite,
					TS:           t.TS,
				}
			}
		}
		if len(in.Verdicts) > 0 {
			payload.Verdicts = make([]client.AgentIngestVerdict, len(in.Verdicts))
			for i, v := range in.Verdicts {
				payload.Verdicts[i] = client.AgentIngestVerdict{
					SpawnID:      v.SpawnID,
					Verdict:      v.Verdict,
					LerneffektMD: v.LerneffektMD,
					TS:           v.TS,
				}
			}
		}
		if len(in.MailboxEdges) > 0 {
			payload.MailboxEdges = make([]client.AgentIngestMailboxEdge, len(in.MailboxEdges))
			for i, e := range in.MailboxEdges {
				payload.MailboxEdges[i] = client.AgentIngestMailboxEdge{
					FromSpawnID: e.FromSpawnID,
					ToSpawnID:   e.ToSpawnID,
					EdgeType:    e.EdgeType,
					TS:          e.TS,
				}
			}
		}

		counts, err := c.AgentsIngest(ctx, payload)
		if err != nil {
			return mcp.NewToolResultError(translateHubError(err)), nil
		}

		body, err := json.Marshal(agentEventResult{Ingested: counts})
		if err != nil {
			return mcp.NewToolResultError(
				fmt.Sprintf("encode response: %v", err),
			), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}
}
