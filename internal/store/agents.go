package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Phase-2d agent-observability persistence layer.
//
// Four typed structs and four Insert* methods write into the schema
// established by migration 0003 (agent_spawns / agent_tokens /
// agent_verdicts / agent_mailbox_edges). All four child-table inserts
// use INSERT OR IGNORE so that the multi-ingest contract (ADR-013) is
// idempotent at the storage layer — a second writer reporting the same
// event from a different source is no-op'd by the UNIQUE-tuple indexes
// rather than producing a constraint error the handler would need to
// translate.
//
// The agent_spawns table is the parent: the writer supplies the
// ULID-shaped id, and a second writer reporting the same spawn (i.e.
// transcript-tail picks up a spawn that an SDK-hook already pushed) is
// silently coalesced via INSERT OR IGNORE on the primary key.
//
// Time semantics: every Insert* method accepts time.Time and stores
// UnixNano(), matching the convention from sessions/events/crashes.
// A zero time.Time falls back to time.Now() — same fallback as
// InsertEvents.
//
// Layering: this file deliberately mirrors the style of UpsertCrash and
// InsertEvents — small Insert methods, no batching today (one row per
// call), no in-method validation of source/verdict/edge_type enum
// values (those are enforced by the CHECK constraints in the schema,
// and rejecting at the handler layer keeps the error message close to
// the wire payload).

// AgentSpawn is one agent-lifecycle row. ID is a writer-supplied
// 26-char ULID-shaped string (see newSessionID for the in-repo helper
// pattern). ParentID and SessionRef are nullable — top-level spawns
// have no parent, standalone QS spawns have no session.
type AgentSpawn struct {
	ID         string
	ParentID   string
	Skill      string
	StartedAt  time.Time
	EndedAt    *time.Time
	Project    string
	SessionRef string
}

// AgentTokenUsage is one token-accounting row. Source must be one of
// 'sdk-hook' / 'transcript' / 'mcp-push' — the schema CHECK constraint
// rejects anything else, the handler is expected to pre-validate so
// the rejection is a 400 not a 500.
type AgentTokenUsage struct {
	SpawnID      string
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheWrite   int64
	TS           time.Time
	Source       string
}

// AgentVerdict is one QS-verdict row attached to a spawn.
// LerneffektMD is optional (NULL when empty).
type AgentVerdict struct {
	SpawnID      string
	Verdict      string
	LerneffektMD string
	TS           time.Time
}

// AgentMailboxEdge is one mailbox-relation row between two spawns.
// FromSpawnID + ToSpawnID + EdgeType + TS is the UNIQUE tuple.
type AgentMailboxEdge struct {
	FromSpawnID string
	ToSpawnID   string
	EdgeType    string
	TS          time.Time
}

// AgentInsertResult reports per-table how many rows were actually
// inserted vs collapsed by the INSERT OR IGNORE idempotency guard.
// A handler returns this in the /agents/ingest response body so
// operators can audit per-call whether a re-push was a no-op.
type AgentInsertResult struct {
	Spawns       int64 `json:"spawns"`
	Tokens       int64 `json:"tokens"`
	Verdicts     int64 `json:"verdicts"`
	MailboxEdges int64 `json:"mailbox_edges"`
}

// InsertAgentSpawn writes (or ignores) a single spawn row. The writer
// is responsible for supplying spawn.ID (ULID-shaped). A repeat-call
// with the same ID is silently a no-op — the multi-ingest contract.
// Returns 1 if the row was inserted, 0 if the PK collided.
func (s *Store) InsertAgentSpawn(ctx context.Context, spawn AgentSpawn) (int64, error) {
	if spawn.ID == "" {
		return 0, fmt.Errorf("store: agent spawn id required")
	}
	if spawn.Skill == "" {
		return 0, fmt.Errorf("store: agent spawn skill required")
	}
	if spawn.Project == "" {
		return 0, fmt.Errorf("store: agent spawn project required")
	}
	started := spawn.StartedAt
	if started.IsZero() {
		started = time.Now()
	}
	var endedAt any
	if spawn.EndedAt != nil {
		endedAt = spawn.EndedAt.UnixNano()
	}
	var parentID, sessionRef any
	if spawn.ParentID != "" {
		parentID = spawn.ParentID
	}
	if spawn.SessionRef != "" {
		sessionRef = spawn.SessionRef
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO agent_spawns(
			id, parent_id, skill, started_at, ended_at, project, session_ref
		) VALUES(?, ?, ?, ?, ?, ?, ?)
	`, spawn.ID, parentID, spawn.Skill, started.UnixNano(), endedAt,
		spawn.Project, sessionRef)
	if err != nil {
		return 0, fmt.Errorf("store: insert agent spawn: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: insert agent spawn rows: %w", err)
	}
	return n, nil
}

// InsertAgentTokens writes (or ignores) a single token-usage row. The
// UNIQUE tuple is (spawn_id, ts, source), so the same token-event
// repeated from the same source is no-op'd, but the same event
// arriving from a different source produces a second row (per-source
// forensics intact — see ADR-013 §Consequences).
//
// The parent spawn must already exist (FK constraint). Callers that
// receive an SDK-hook spawn-begin event typically InsertAgentSpawn
// first, then InsertAgentTokens for the same call.
func (s *Store) InsertAgentTokens(ctx context.Context, tu AgentTokenUsage) (int64, error) {
	if tu.SpawnID == "" {
		return 0, fmt.Errorf("store: agent tokens spawn_id required")
	}
	if tu.Source == "" {
		return 0, fmt.Errorf("store: agent tokens source required")
	}
	ts := tu.TS
	if ts.IsZero() {
		ts = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO agent_tokens(
			spawn_id, input_tokens, output_tokens, cache_read, cache_write, ts, source
		) VALUES(?, ?, ?, ?, ?, ?, ?)
	`, tu.SpawnID, tu.InputTokens, tu.OutputTokens, tu.CacheRead,
		tu.CacheWrite, ts.UnixNano(), tu.Source)
	if err != nil {
		return 0, fmt.Errorf("store: insert agent tokens: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: insert agent tokens rows: %w", err)
	}
	return n, nil
}

// InsertAgentVerdict writes (or ignores) a single QS verdict. UNIQUE
// tuple is (spawn_id, verdict, ts) — same verdict reported by two
// sources at the same ts is dedup'd (semantic-identity not
// source-differentiated for verdicts, see ADR-013 §Consequences).
func (s *Store) InsertAgentVerdict(ctx context.Context, v AgentVerdict) (int64, error) {
	if v.SpawnID == "" {
		return 0, fmt.Errorf("store: agent verdict spawn_id required")
	}
	if v.Verdict == "" {
		return 0, fmt.Errorf("store: agent verdict required")
	}
	ts := v.TS
	if ts.IsZero() {
		ts = time.Now()
	}
	var lerneffekt any
	if v.LerneffektMD != "" {
		lerneffekt = v.LerneffektMD
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO agent_verdicts(
			spawn_id, verdict, lerneffekt_md, ts
		) VALUES(?, ?, ?, ?)
	`, v.SpawnID, v.Verdict, lerneffekt, ts.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("store: insert agent verdict: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: insert agent verdict rows: %w", err)
	}
	return n, nil
}

// InsertAgentMailboxEdge writes (or ignores) a single mailbox-relation
// row. UNIQUE tuple is (from_spawn_id, to_spawn_id, edge_type, ts).
func (s *Store) InsertAgentMailboxEdge(ctx context.Context, e AgentMailboxEdge) (int64, error) {
	if e.FromSpawnID == "" || e.ToSpawnID == "" {
		return 0, fmt.Errorf("store: agent mailbox edge spawn ids required")
	}
	if e.EdgeType == "" {
		return 0, fmt.Errorf("store: agent mailbox edge type required")
	}
	ts := e.TS
	if ts.IsZero() {
		ts = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO agent_mailbox_edges(
			from_spawn_id, to_spawn_id, edge_type, ts
		) VALUES(?, ?, ?, ?)
	`, e.FromSpawnID, e.ToSpawnID, e.EdgeType, ts.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("store: insert agent mailbox edge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: insert agent mailbox edge rows: %w", err)
	}
	return n, nil
}

// ensureFKError is a small ergonomic helper for callers that want to
// distinguish "spawn parent not found" from "real DB problem". SQLite
// reports FK violations as a generic error string — we surface it via
// errors.Is(err, sql.ErrNoRows) for callers that want to translate
// to 404. Today no caller needs this, but the hook is reserved for
// the read-endpoints in S3+.
var _ = sql.ErrNoRows // keep the import live for future read methods
