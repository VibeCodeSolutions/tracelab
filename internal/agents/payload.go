// Package agents owns the HTTP-surface for the Phase-2d agent
// observability domain (see docs/ARCH.md §Phase 2d and ADR-013).
//
// One handler, one route: POST /agents/ingest accepts a multi-source
// JSON payload (sdk-hook / transcript / mcp-push) and persists into
// agent_spawns / agent_tokens / agent_verdicts / agent_mailbox_edges.
// Idempotency is enforced at the storage layer via INSERT OR IGNORE on
// UNIQUE-tuple indexes — the handler does NOT need to dedupe; it
// trusts the schema.
//
// The package deliberately stays slim in S1: read-views
// (/agents/sessions, /agents/tree/{id}, /agents/tokens,
// /agents/verdicts) land in S3+ and live in this package alongside
// the ingest handler.
package agents

import (
	"encoding/json"
	"fmt"
	"time"
)

// Source identifies which of the three ingest paths a payload arrived
// from. The schema CHECK on agent_tokens.source enforces the same
// vocabulary; the handler pre-validates so a bad source produces a
// 400 instead of a 500.
type Source string

const (
	SourceSDKHook    Source = "sdk-hook"
	SourceTranscript Source = "transcript"
	SourceMCPPush    Source = "mcp-push"
)

// validSources is the canonical allow-list. Keep this in sync with the
// CHECK constraint in migrations/0003_agents_schema.up.sql and the
// ADR-013 §Consequences §Wire-compat statement.
var validSources = map[Source]bool{
	SourceSDKHook:    true,
	SourceTranscript: true,
	SourceMCPPush:    true,
}

// validVerdicts mirrors the schema CHECK on agent_verdicts.verdict.
var validVerdicts = map[string]bool{
	"freigabe":   true,
	"auflagen":   true,
	"rueckgabe":  true,
	"eskalation": true,
	"none":       true,
}

// validEdgeTypes mirrors the schema CHECK on agent_mailbox_edges.edge_type.
var validEdgeTypes = map[string]bool{
	"spawn":    true,
	"return":   true,
	"escalate": true,
	"delegate": true,
}

// validEventRefTypes mirrors the schema CHECK on agent_event_refs.ref_type.
// Phase 2d S5-Tail (ADR-014 Accepted, Option B).
var validEventRefTypes = map[string]bool{
	"observed":  true,
	"context":   true,
	"caused-by": true,
}

// IngestPayload is the wire-level JSON the /agents/ingest endpoint
// accepts. All sub-records are optional per source — a transcript-tail
// often carries only `tokens` + `verdict` for an existing spawn; an
// SDK-hook Stop event carries the full lifecycle; an MCP-push from
// the worker itself carries whatever the worker reports.
//
// The `source` discriminator is required and shapes the schema
// dispatch (see ADR-013 §Consequences §Wire-compat statement).
type IngestPayload struct {
	Source       Source               `json:"source"`
	Spawn        *SpawnPayload        `json:"spawn,omitempty"`
	Tokens       []TokensPayload      `json:"tokens,omitempty"`
	Verdicts     []VerdictPayload     `json:"verdicts,omitempty"`
	MailboxEdges []MailboxEdgePayload `json:"mailbox_edges,omitempty"`
	// EventRefs added Phase 2d S5-Tail (ADR-014 Accepted, Option B).
	// Cross-domain bridge from a spawn to an events row from the
	// app-log domain. Additive to the wire envelope — older writers
	// simply omit the field.
	EventRefs []EventRefPayload `json:"event_refs,omitempty"`
}

// SpawnPayload is a single agent_spawns row. ID is writer-supplied
// (ULID-shaped 26-char string). Project + Skill + StartedAt are
// required; the rest is optional.
type SpawnPayload struct {
	ID         string `json:"id"`
	ParentID   string `json:"parent_id,omitempty"`
	Skill      string `json:"skill"`
	StartedAt  int64  `json:"started_at"`
	EndedAt    *int64 `json:"ended_at,omitempty"`
	Project    string `json:"project"`
	SessionRef string `json:"session_ref,omitempty"`
}

// TokensPayload is a single agent_tokens delta. SpawnID + TS are
// required; the four counters default to 0. Source is taken from the
// envelope's IngestPayload.Source, NOT repeated per row (the wire
// shape is cleaner that way; the storage layer fills it in).
type TokensPayload struct {
	SpawnID      string `json:"spawn_id"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheWrite   int64  `json:"cache_write"`
	TS           int64  `json:"ts"`
}

// VerdictPayload is a single agent_verdicts row. SpawnID + Verdict +
// TS are required; LerneffektMD is the optional follow-up note.
type VerdictPayload struct {
	SpawnID      string `json:"spawn_id"`
	Verdict      string `json:"verdict"`
	LerneffektMD string `json:"lerneffekt_md,omitempty"`
	TS           int64  `json:"ts"`
}

// MailboxEdgePayload is a single agent_mailbox_edges row. All four
// fields are required.
type MailboxEdgePayload struct {
	FromSpawnID string `json:"from_spawn_id"`
	ToSpawnID   string `json:"to_spawn_id"`
	EdgeType    string `json:"edge_type"`
	TS          int64  `json:"ts"`
}

// EventRefPayload is a single agent_event_refs row (Phase 2d S5-Tail,
// ADR-014 Accepted, Option B). All four fields are required.
//
//   - SpawnID  → FK to agent_spawns.id (ULID-shaped writer-supplied)
//   - EventID  → FK to events.id (AUTOINCREMENT integer from migration 0001)
//   - RefType  → one of "observed" | "context" | "caused-by"
//   - TS       → unix-nano timestamp of the reference (matches the
//                contract on tokens/verdicts/edges)
type EventRefPayload struct {
	SpawnID string `json:"spawn_id"`
	EventID int64  `json:"event_id"`
	RefType string `json:"ref_type"`
	TS      int64  `json:"ts"`
}

// validate runs the source-and-enum-vocabulary checks that the handler
// promises to do BEFORE touching the database. A nil error means the
// payload is shape-safe (handler can proceed with persistence); a
// non-nil error is shaped for a 400 response.
//
// validate intentionally does NOT check FK existence (parent_id,
// session_ref) — that is the FK constraint's job, and a missing
// parent is a real (not user-facing) error worth surfacing as 500
// rather than 400.
func (p *IngestPayload) validate() error {
	if p.Source == "" {
		return fmt.Errorf("source required")
	}
	if !validSources[p.Source] {
		return fmt.Errorf("unknown source %q (want one of sdk-hook|transcript|mcp-push)", p.Source)
	}
	if p.Spawn == nil && len(p.Tokens) == 0 && len(p.Verdicts) == 0 && len(p.MailboxEdges) == 0 && len(p.EventRefs) == 0 {
		return fmt.Errorf("payload is empty (need at least one of spawn|tokens|verdicts|mailbox_edges|event_refs)")
	}
	if p.Spawn != nil {
		if p.Spawn.ID == "" {
			return fmt.Errorf("spawn.id required")
		}
		if p.Spawn.Skill == "" {
			return fmt.Errorf("spawn.skill required")
		}
		if p.Spawn.Project == "" {
			return fmt.Errorf("spawn.project required")
		}
		if p.Spawn.StartedAt <= 0 {
			return fmt.Errorf("spawn.started_at required (unix-nano > 0)")
		}
	}
	for i, t := range p.Tokens {
		if t.SpawnID == "" {
			return fmt.Errorf("tokens[%d].spawn_id required", i)
		}
		if t.TS <= 0 {
			return fmt.Errorf("tokens[%d].ts required (unix-nano > 0)", i)
		}
	}
	for i, v := range p.Verdicts {
		if v.SpawnID == "" {
			return fmt.Errorf("verdicts[%d].spawn_id required", i)
		}
		if !validVerdicts[v.Verdict] {
			return fmt.Errorf("verdicts[%d].verdict %q invalid (want freigabe|auflagen|rueckgabe|eskalation|none)", i, v.Verdict)
		}
		if v.TS <= 0 {
			return fmt.Errorf("verdicts[%d].ts required (unix-nano > 0)", i)
		}
	}
	for i, e := range p.MailboxEdges {
		if e.FromSpawnID == "" || e.ToSpawnID == "" {
			return fmt.Errorf("mailbox_edges[%d].{from,to}_spawn_id required", i)
		}
		if !validEdgeTypes[e.EdgeType] {
			return fmt.Errorf("mailbox_edges[%d].edge_type %q invalid (want spawn|return|escalate|delegate)", i, e.EdgeType)
		}
		if e.TS <= 0 {
			return fmt.Errorf("mailbox_edges[%d].ts required (unix-nano > 0)", i)
		}
	}
	for i, er := range p.EventRefs {
		if er.SpawnID == "" {
			return fmt.Errorf("event_refs[%d].spawn_id required", i)
		}
		if er.EventID <= 0 {
			return fmt.Errorf("event_refs[%d].event_id required (positive integer FK to events.id)", i)
		}
		if !validEventRefTypes[er.RefType] {
			return fmt.Errorf("event_refs[%d].ref_type %q invalid (want observed|context|caused-by)", i, er.RefType)
		}
		if er.TS <= 0 {
			return fmt.Errorf("event_refs[%d].ts required (unix-nano > 0)", i)
		}
	}
	return nil
}

// decodeIngest is a small wrapper around json.Decoder with
// DisallowUnknownFields so a typo on the wire side surfaces as a 400
// instead of silently being dropped. Mirrors internal/http.decodeJSON.
func decodeIngest(r interface {
	Read(p []byte) (int, error)
}) (IngestPayload, error) {
	var p IngestPayload
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return IngestPayload{}, err
	}
	return p, nil
}

// nsToTime is a tiny helper local to this package (instead of pulling
// in a util module). The agent payload uses unix-nano consistently —
// same convention as sessions/events/crashes wire-shapes.
func nsToTime(ns int64) time.Time {
	if ns <= 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}
