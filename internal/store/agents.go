package store

import (
	"context"
	"database/sql"
	"errors"
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

// -----------------------------------------------------------------------------
// Phase 2d S4 — Read-surface for the dashboard "Agents" tab.
//
// Five additive read methods drive the four /agents/* read endpoints
// (see internal/agents/read.go) plus the dashboard tab body
// (internal/dashboard/agents.go):
//
//   - ListAgentSpawns          — paginated newest-first list of spawns,
//                                optional project / session filters
//   - CountAgentSpawns         — total matching ListAgentSpawns filter
//   - AgentSpawnByID           — single-spawn lookup (sql.ErrNoRows when unknown)
//   - ListAgentSpawnTree       — root + all transitive descendants
//   - AgentTokensBySpawn       — all token-rows (per-source preserved)
//   - AgentVerdictsBySpawn     — all verdict-rows
//
// All time fields are stored as unix-nano (column type INTEGER); the
// methods convert to/from time.Time at the boundary, same convention as
// sessions / events / crashes.
//
// Tree-reconstruction strategy: application-level walk over a flat
// SELECT (root + every row whose parent chain reaches root). SQLite's
// `WITH RECURSIVE` works, but a typical agent tree is 10-100 nodes —
// the flat-then-walk approach keeps the SQL simple, the test trivial,
// and avoids a CTE-syntax dependency on a future store-layer migration.
// Document choice in docs/ARCH.md §Phase 2d Read-Surface.

// AgentSpawnRow is one agent_spawns row returned by the read methods.
// ParentID / EndedAt / SessionRef are pointers so callers can
// distinguish "not set" from zero-value. All times are time.Time;
// caller decides on rendering.
type AgentSpawnRow struct {
	ID         string
	ParentID   *string
	Skill      string
	StartedAt  time.Time
	EndedAt    *time.Time
	Project    string
	SessionRef *string
}

// ListAgentSpawnsOpts bundles optional filter / pagination parameters
// for ListAgentSpawns and CountAgentSpawns. Zero values map to "no
// filter, no offset, default limit, newest first". The sort is hard-
// coded started_at DESC for now — the agents tab is a triage surface,
// reverse-chronological is the natural reading order, so the typed
// SessionSort/CrashSort enum machinery isn't worth it yet.
type ListAgentSpawnsOpts struct {
	Limit            int
	Offset           int
	FilterProject    string
	FilterSessionRef string
}

// ListAgentSpawns returns up to opts.Limit spawn rows ordered newest
// first (started_at DESC, id DESC as tiebreaker). Empty result is
// `nil, nil` — same convention as the session/crash list reads.
//
// limit <= 0 falls back to a 50 default. opts.Offset < 0 is clamped
// to 0 to spare the SQL layer a defensive check.
func (s *Store) ListAgentSpawns(ctx context.Context, opts ListAgentSpawnsOpts) ([]AgentSpawnRow, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	const baseSelect = `
		SELECT id, parent_id, skill, started_at, ended_at, project, session_ref
		FROM agent_spawns
	`
	var (
		query string
		args  []any
	)
	switch {
	case opts.FilterProject != "" && opts.FilterSessionRef != "":
		query = baseSelect + ` WHERE project = ? AND session_ref = ?
			ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`
		args = []any{opts.FilterProject, opts.FilterSessionRef, limit, offset}
	case opts.FilterProject != "":
		query = baseSelect + ` WHERE project = ?
			ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`
		args = []any{opts.FilterProject, limit, offset}
	case opts.FilterSessionRef != "":
		query = baseSelect + ` WHERE session_ref = ?
			ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`
		args = []any{opts.FilterSessionRef, limit, offset}
	default:
		query = baseSelect + ` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`
		args = []any{limit, offset}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list agent spawns: %w", err)
	}
	defer rows.Close()

	var out []AgentSpawnRow
	for rows.Next() {
		row, err := scanAgentSpawnRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list agent spawns rows: %w", err)
	}
	return out, nil
}

// CountAgentSpawns returns the total number of spawn rows matching the
// same filters ListAgentSpawns honours. Used by the dashboard tab to
// drive pagination counters.
func (s *Store) CountAgentSpawns(ctx context.Context, filterProject, filterSessionRef string) (int64, error) {
	var (
		query string
		args  []any
	)
	switch {
	case filterProject != "" && filterSessionRef != "":
		query = `SELECT COUNT(*) FROM agent_spawns WHERE project = ? AND session_ref = ?`
		args = []any{filterProject, filterSessionRef}
	case filterProject != "":
		query = `SELECT COUNT(*) FROM agent_spawns WHERE project = ?`
		args = []any{filterProject}
	case filterSessionRef != "":
		query = `SELECT COUNT(*) FROM agent_spawns WHERE session_ref = ?`
		args = []any{filterSessionRef}
	default:
		query = `SELECT COUNT(*) FROM agent_spawns`
	}
	var n int64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count agent spawns: %w", err)
	}
	return n, nil
}

// AgentSpawnByID returns the spawn row for the given id, or
// sql.ErrNoRows when no spawn matches. The caller is expected to
// translate sql.ErrNoRows → HTTP 404.
func (s *Store) AgentSpawnByID(ctx context.Context, id string) (AgentSpawnRow, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, parent_id, skill, started_at, ended_at, project, session_ref
		FROM agent_spawns
		WHERE id = ?
	`, id)
	res, err := scanAgentSpawnRowFromRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentSpawnRow{}, sql.ErrNoRows
		}
		return AgentSpawnRow{}, fmt.Errorf("store: agent spawn by id: %w", err)
	}
	return res, nil
}

// ListAgentSpawnTree returns the root spawn plus every transitive
// descendant whose parent chain reaches rootID. The returned slice is
// ordered breadth-first from the root (root first, then children, then
// grand-children, …), with siblings sorted by started_at ASC. This
// matches the natural reading order in a tree view — parents before
// children, oldest sibling first.
//
// Strategy: load the whole project's spawn rows in one query (in
// practice 10-100 rows per project; an index on project keeps this
// cheap), then walk parent→children pointers in-memory. Avoids the
// SQLite-WITH-RECURSIVE syntax footprint and keeps the unit test
// trivial. If a future deployment ships realistic projects with >10k
// spawns per project, revisit via ADR — but for the single-user dev
// surface this is more than adequate.
//
// rootID must be non-empty. An unknown id returns sql.ErrNoRows.
func (s *Store) ListAgentSpawnTree(ctx context.Context, rootID string) ([]AgentSpawnRow, error) {
	if rootID == "" {
		return nil, fmt.Errorf("store: list agent spawn tree: root id required")
	}

	// Step 1: look up the root to (a) confirm existence and (b) learn
	// which project to scope the children query to. Scoping by project
	// keeps the working set bounded even on a shared NTFS DB with many
	// concurrent agent projects.
	root, err := s.AgentSpawnByID(ctx, rootID)
	if err != nil {
		return nil, err
	}

	// Step 2: pull every spawn in the same project. We could narrow by
	// "descendants only" via WITH RECURSIVE, but at this dataset size
	// a flat query + in-memory walk is cheaper to write and test.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, parent_id, skill, started_at, ended_at, project, session_ref
		FROM agent_spawns
		WHERE project = ?
		ORDER BY started_at ASC, id ASC
	`, root.Project)
	if err != nil {
		return nil, fmt.Errorf("store: list agent spawn tree query: %w", err)
	}
	defer rows.Close()

	byID := make(map[string]AgentSpawnRow)
	childrenByParent := make(map[string][]string)
	for rows.Next() {
		r, err := scanAgentSpawnRow(rows)
		if err != nil {
			return nil, err
		}
		byID[r.ID] = r
		if r.ParentID != nil && *r.ParentID != "" {
			childrenByParent[*r.ParentID] = append(childrenByParent[*r.ParentID], r.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list agent spawn tree rows: %w", err)
	}

	// Step 3: BFS walk from root. Siblings are already in started_at ASC
	// order thanks to the SELECT ORDER BY.
	out := make([]AgentSpawnRow, 0, len(byID))
	queue := []string{rootID}
	visited := make(map[string]bool, len(byID))
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		if visited[head] {
			// Defensive: parent_id is FK-self-referential and the schema
			// would allow a malformed cycle (no CHECK against it). A
			// visited-set keeps the walk terminating even in that case.
			continue
		}
		visited[head] = true
		row, ok := byID[head]
		if !ok {
			// head is outside the project scope (or unknown) — skip but
			// don't error. The root was already verified by AgentSpawnByID.
			continue
		}
		out = append(out, row)
		queue = append(queue, childrenByParent[head]...)
	}
	return out, nil
}

// AgentTokenRow is one agent_tokens row returned by the read methods.
// Source is preserved so the caller can render per-source breakdowns
// (multi-ingest forensic view, ADR-013 §Consequences).
type AgentTokenRow struct {
	ID           int64
	SpawnID      string
	InputTokens  int64
	OutputTokens int64
	CacheRead    int64
	CacheWrite   int64
	TS           time.Time
	Source       string
}

// AgentTokensBySpawn returns every token-row for the given spawn,
// ordered ascending by ts (and id as tiebreaker). All three sources
// are returned interleaved — the caller aggregates and slices by
// source as needed. Empty result is `nil, nil`.
func (s *Store) AgentTokensBySpawn(ctx context.Context, spawnID string) ([]AgentTokenRow, error) {
	if spawnID == "" {
		return nil, fmt.Errorf("store: agent tokens by spawn: id required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, spawn_id, input_tokens, output_tokens, cache_read, cache_write, ts, source
		FROM agent_tokens
		WHERE spawn_id = ?
		ORDER BY ts ASC, id ASC
	`, spawnID)
	if err != nil {
		return nil, fmt.Errorf("store: agent tokens by spawn: %w", err)
	}
	defer rows.Close()

	var out []AgentTokenRow
	for rows.Next() {
		var r AgentTokenRow
		var tsNano int64
		if err := rows.Scan(&r.ID, &r.SpawnID, &r.InputTokens, &r.OutputTokens,
			&r.CacheRead, &r.CacheWrite, &tsNano, &r.Source); err != nil {
			return nil, fmt.Errorf("store: scan agent token: %w", err)
		}
		r.TS = time.Unix(0, tsNano)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: agent tokens by spawn rows: %w", err)
	}
	return out, nil
}

// AgentVerdictRow is one agent_verdicts row returned by the read methods.
// LerneffektMD is empty when the source row carried NULL — the database
// distinguishes NULL from empty string, but the wire shape collapses
// both to the empty string (consumers care only about "is there a note").
type AgentVerdictRow struct {
	ID           int64
	SpawnID      string
	Verdict      string
	LerneffektMD string
	TS           time.Time
}

// AgentVerdictsBySpawn returns every verdict row attached to the spawn,
// ordered ascending by ts (and id as tiebreaker). Empty result is
// `nil, nil`.
func (s *Store) AgentVerdictsBySpawn(ctx context.Context, spawnID string) ([]AgentVerdictRow, error) {
	if spawnID == "" {
		return nil, fmt.Errorf("store: agent verdicts by spawn: id required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, spawn_id, verdict, lerneffekt_md, ts
		FROM agent_verdicts
		WHERE spawn_id = ?
		ORDER BY ts ASC, id ASC
	`, spawnID)
	if err != nil {
		return nil, fmt.Errorf("store: agent verdicts by spawn: %w", err)
	}
	defer rows.Close()

	var out []AgentVerdictRow
	for rows.Next() {
		var r AgentVerdictRow
		var tsNano int64
		var lerneffekt sql.NullString
		if err := rows.Scan(&r.ID, &r.SpawnID, &r.Verdict, &lerneffekt, &tsNano); err != nil {
			return nil, fmt.Errorf("store: scan agent verdict: %w", err)
		}
		r.TS = time.Unix(0, tsNano)
		if lerneffekt.Valid {
			r.LerneffektMD = lerneffekt.String
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: agent verdicts by spawn rows: %w", err)
	}
	return out, nil
}

// scanAgentSpawnRow extracts one AgentSpawnRow from a *sql.Rows cursor.
// Shared between ListAgentSpawns and ListAgentSpawnTree.
func scanAgentSpawnRow(rows *sql.Rows) (AgentSpawnRow, error) {
	var (
		r          AgentSpawnRow
		parentID   sql.NullString
		startedNano int64
		endedNano  sql.NullInt64
		sessionRef sql.NullString
	)
	if err := rows.Scan(&r.ID, &parentID, &r.Skill, &startedNano, &endedNano, &r.Project, &sessionRef); err != nil {
		return AgentSpawnRow{}, fmt.Errorf("store: scan agent spawn: %w", err)
	}
	r.StartedAt = time.Unix(0, startedNano)
	if parentID.Valid {
		v := parentID.String
		r.ParentID = &v
	}
	if endedNano.Valid {
		t := time.Unix(0, endedNano.Int64)
		r.EndedAt = &t
	}
	if sessionRef.Valid {
		v := sessionRef.String
		r.SessionRef = &v
	}
	return r, nil
}

// scanAgentSpawnRowFromRow is the *sql.Row variant of scanAgentSpawnRow
// — used by single-row lookups (AgentSpawnByID) which receive *sql.Row
// rather than *sql.Rows. The two scan APIs are nearly identical but
// .Scan errors map differently (Row returns sql.ErrNoRows directly).
func scanAgentSpawnRowFromRow(row *sql.Row) (AgentSpawnRow, error) {
	var (
		r           AgentSpawnRow
		parentID    sql.NullString
		startedNano int64
		endedNano   sql.NullInt64
		sessionRef  sql.NullString
	)
	if err := row.Scan(&r.ID, &parentID, &r.Skill, &startedNano, &endedNano, &r.Project, &sessionRef); err != nil {
		return AgentSpawnRow{}, err
	}
	r.StartedAt = time.Unix(0, startedNano)
	if parentID.Valid {
		v := parentID.String
		r.ParentID = &v
	}
	if endedNano.Valid {
		t := time.Unix(0, endedNano.Int64)
		r.EndedAt = &t
	}
	if sessionRef.Valid {
		v := sessionRef.String
		r.SessionRef = &v
	}
	return r, nil
}
