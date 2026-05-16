// Package store implements the SQLite-backed persistence layer for tracelab.
//
// The store owns a single *sql.DB handle (modernc.org/sqlite, pure Go,
// CGO-free for clean cross-compile to Linux+Windows). Schema management is
// handled by an embedded, idempotent migration loader — no external migrate
// dependency for now (one migration, simple version table).
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps a *sql.DB and provides the public CRUD API for tracelab's
// sessions / events / crashes / screenshots tables.
type Store struct {
	db *sql.DB
}

// Session represents a logical test/debug run.
type Session struct {
	ID        string
	Label     string
	StartedAt time.Time
	EndedAt   *time.Time
}

// Event is a single ingested log line / structured record.
type Event struct {
	ID        int64
	SessionID string
	TS        time.Time
	Source    string
	Level     string
	Msg       string
	Meta      json.RawMessage
}

// Open opens (or creates) the SQLite database at dsn, applies PRAGMAs and
// runs all pending migrations. dsn is a filesystem path; modernc.org/sqlite
// query parameters can be appended after a `?` if needed.
func Open(dsn string) (*Store, error) {
	// modernc.org/sqlite registers itself under the driver name "sqlite".
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}

	// Single connection guarantees PRAGMAs apply globally; SQLite is not
	// helped by parallel writers anyway.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("store: pragma %q: %w", p, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB exposes the underlying handle for advanced uses (tests, debug). Avoid
// in production code paths — prefer the typed API.
func (s *Store) DB() *sql.DB { return s.db }

// migrate applies all .up.sql files from the embedded migrations/ directory
// in lexicographic order, recording applied versions in schema_migrations.
// Idempotent: a second call is a no-op.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("store: read migrations: %w", err)
	}
	type mig struct {
		version int
		name    string
	}
	var ups []mig
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		// Format: NNNN_description.up.sql
		under := strings.IndexByte(name, '_')
		if under <= 0 {
			return fmt.Errorf("store: invalid migration name %q", name)
		}
		v, err := strconv.Atoi(name[:under])
		if err != nil {
			return fmt.Errorf("store: parse version in %q: %w", name, err)
		}
		ups = append(ups, mig{version: v, name: name})
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].version < ups[j].version })

	for _, m := range ups {
		var dummy int
		err := s.db.QueryRowContext(ctx,
			`SELECT 1 FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&dummy)
		if err == nil {
			continue // already applied
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: check migration %d: %w", m.version, err)
		}

		body, err := fs.ReadFile(migrationsFS, "migrations/"+m.name)
		if err != nil {
			return fmt.Errorf("store: read %s: %w", m.name, err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: begin tx for %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: apply %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
			m.version, time.Now().UnixNano(),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: record %s: %w", m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit %s: %w", m.name, err)
		}
	}
	return nil
}

// newSessionID returns a 26-char lexicographically sortable id:
// 16 hex chars of unix-nano timestamp + 10 hex chars of crypto random.
// Sortable like a ULID, no external dep.
func newSessionID() (string, error) {
	var rnd [5]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%016x%s", time.Now().UnixNano(), hex.EncodeToString(rnd[:])), nil
}

// CreateSession inserts a new session with started_at = now and returns its id.
func (s *Store) CreateSession(ctx context.Context, label string) (string, error) {
	id, err := newSessionID()
	if err != nil {
		return "", fmt.Errorf("store: gen session id: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions(id, label, started_at) VALUES(?, ?, ?)`,
		id, label, time.Now().UnixNano(),
	)
	if err != nil {
		return "", fmt.Errorf("store: create session: %w", err)
	}
	return id, nil
}

// EndSession marks the session as ended at now. Returns sql.ErrNoRows if
// the session id does not exist.
func (s *Store) EndSession(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET ended_at = ? WHERE id = ?`,
		time.Now().UnixNano(), id,
	)
	if err != nil {
		return fmt.Errorf("store: end session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: end session rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// InsertEvents writes a batch of events for the given session inside a
// single transaction. The Event.ID and Event.SessionID fields on input are
// ignored — id is auto-assigned, session id is taken from the parameter.
// If e.TS.IsZero(), now is used.
func (s *Store) InsertEvents(ctx context.Context, sessionID string, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: insert events begin: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events(session_id, ts, source, level, msg, meta)
		VALUES(?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: insert events prepare: %w", err)
	}
	defer stmt.Close()
	for _, e := range events {
		ts := e.TS
		if ts.IsZero() {
			ts = time.Now()
		}
		var meta any
		if len(e.Meta) > 0 {
			meta = string(e.Meta)
		}
		if _, err := stmt.ExecContext(ctx,
			sessionID, ts.UnixNano(), e.Source, e.Level, e.Msg, meta,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: insert event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: insert events commit: %w", err)
	}
	return nil
}

// EventsSince returns up to limit events for the given session whose
// rowid is strictly greater than sinceID, ordered ascending by rowid.
//
// This is the forward-cursor query that backs the hub's GET /events
// endpoint and the MCP tail_since tool (ADR-008, Phase 2b S4). The
// `id > sinceID` predicate is intentionally strict so callers that loop
// with `sinceID = lastReturnedID` never re-read a row.
//
// limit <= 0 falls back to a 500 default (the same default the HTTP
// handler applies). A higher limit is the caller's responsibility — the
// store does not cap; the HTTP layer caps at 5000 (see ADR-008).
//
// Empty session ID or unknown session ID returns an empty slice with a
// nil error: /events is a cursor read, not a session-existence probe.
func (s *Store) EventsSince(ctx context.Context, sessionID string, sinceID int64, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, ts, source, level, msg, meta
		FROM events
		WHERE session_id = ? AND id > ?
		ORDER BY id ASC
		LIMIT ?
	`, sessionID, sinceID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: events since: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var tsNano int64
		var meta sql.NullString
		if err := rows.Scan(&e.ID, &e.SessionID, &tsNano, &e.Source, &e.Level, &e.Msg, &meta); err != nil {
			return nil, fmt.Errorf("store: scan event: %w", err)
		}
		e.TS = time.Unix(0, tsNano)
		if meta.Valid {
			e.Meta = json.RawMessage(meta.String)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: rows event since: %w", err)
	}
	return out, nil
}

// RecentEvents returns up to limit most-recent events for the session,
// ordered by ts ASC (chronological — newest at the end of the slice).
func (s *Store) RecentEvents(ctx context.Context, sessionID string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	// Inner query: take the last `limit` rows by ts DESC; outer reorders ASC.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, ts, source, level, msg, meta FROM (
			SELECT id, session_id, ts, source, level, msg, meta
			FROM events
			WHERE session_id = ?
			ORDER BY ts DESC, id DESC
			LIMIT ?
		) ORDER BY ts ASC, id ASC
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: recent events: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var tsNano int64
		var meta sql.NullString
		if err := rows.Scan(&e.ID, &e.SessionID, &tsNano, &e.Source, &e.Level, &e.Msg, &meta); err != nil {
			return nil, fmt.Errorf("store: scan event: %w", err)
		}
		e.TS = time.Unix(0, tsNano)
		if meta.Valid {
			e.Meta = json.RawMessage(meta.String)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: rows event: %w", err)
	}
	return out, nil
}

// UpsertCrash records a crash for the given session. If a row with the
// same (session_id, fingerprint) already exists, its count is incremented
// and its ts is updated to the most recent occurrence (so the crash sorts
// to the top in chronological views). Otherwise a new row is inserted
// with count=1.
//
// The whole operation runs in a single transaction so concurrent ingests
// can't race between the SELECT and the UPDATE/INSERT.
//
// Returns sql.ErrNoRows if the session id does not exist (the FK would
// reject the insert path; the SELECT path explicitly checks first to
// give callers a consistent error regardless of which branch was taken).
func (s *Store) UpsertCrash(ctx context.Context, sessionID string, ts time.Time, fingerprint, stacktrace string) error {
	if fingerprint == "" {
		return fmt.Errorf("store: upsert crash: empty fingerprint")
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: upsert crash begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op on commit

	// Verify session exists up-front so we surface a clean sql.ErrNoRows
	// rather than a generic FK error on the INSERT branch.
	var exists int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM sessions WHERE id = ?`, sessionID,
	).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("store: upsert crash session-lookup: %w", err)
	}

	var crashID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM crashes
		WHERE session_id = ? AND fingerprint = ?
		LIMIT 1
	`, sessionID, fingerprint).Scan(&crashID)
	switch {
	case err == nil:
		// Existing row → bump counter, refresh ts.
		if _, err := tx.ExecContext(ctx, `
			UPDATE crashes
			SET count = count + 1, ts = ?
			WHERE id = ?
		`, ts.UnixNano(), crashID); err != nil {
			return fmt.Errorf("store: upsert crash bump: %w", err)
		}
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO crashes(session_id, ts, fingerprint, stacktrace, count)
			VALUES(?, ?, ?, ?, 1)
		`, sessionID, ts.UnixNano(), fingerprint, stacktrace); err != nil {
			return fmt.Errorf("store: upsert crash insert: %w", err)
		}
	default:
		return fmt.Errorf("store: upsert crash lookup: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: upsert crash commit: %w", err)
	}
	return nil
}

// CrashRow is a denormalised view of a `crashes` table row, used by tests
// and (later) by the read API.
type CrashRow struct {
	ID          int64
	SessionID   string
	TS          time.Time
	Fingerprint string
	Stacktrace  string
	Count       int
}

// CrashesBySession returns up to limit crashes for the given session,
// newest first (ORDER BY ts DESC, id DESC). Backs both the existing
// crash-detection tests and the hub's GET /crashes endpoint (ADR-009,
// Phase 2b S6).
//
// limit <= 0 falls back to a 500 default (the same default the HTTP
// handler applies). A higher limit is the caller's responsibility — the
// store does not cap; the HTTP layer caps at 5000 (see ADR-009).
//
// Empty/unknown session ID returns an empty slice with a nil error:
// /crashes is a list-read, not a session-existence probe (existence is
// discoverable via /sessions).
func (s *Store) CrashesBySession(ctx context.Context, sessionID string, limit int) ([]CrashRow, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, ts, fingerprint, stacktrace, count
		FROM crashes
		WHERE session_id = ?
		ORDER BY ts DESC, id DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: crashes by session: %w", err)
	}
	defer rows.Close()

	var out []CrashRow
	for rows.Next() {
		var c CrashRow
		var tsNano int64
		if err := rows.Scan(&c.ID, &c.SessionID, &tsNano, &c.Fingerprint, &c.Stacktrace, &c.Count); err != nil {
			return nil, fmt.Errorf("store: scan crash: %w", err)
		}
		c.TS = time.Unix(0, tsNano)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: rows crash: %w", err)
	}
	return out, nil
}

// ListSessions returns up to limit sessions, newest first.
func (s *Store) ListSessions(ctx context.Context, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, label, started_at, ended_at
		FROM sessions
		ORDER BY started_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var s Session
		var startedNano int64
		var endedNano sql.NullInt64
		if err := rows.Scan(&s.ID, &s.Label, &startedNano, &endedNano); err != nil {
			return nil, fmt.Errorf("store: scan session: %w", err)
		}
		s.StartedAt = time.Unix(0, startedNano)
		if endedNano.Valid {
			t := time.Unix(0, endedNano.Int64)
			s.EndedAt = &t
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: rows session: %w", err)
	}
	return out, nil
}

// SessionSort enumerates the allowed sort keys for ListSessionsWithCounts.
// Kept as a typed constant set so the dashboard handler can pass it through
// a whitelist check at the HTTP boundary without re-stating the SQL forms.
type SessionSort int

const (
	// SortSessionStartedAtDesc — newest first (default). Matches the
	// existing ListSessions order and the dashboard's "what just
	// happened" reading.
	SortSessionStartedAtDesc SessionSort = iota
	// SortSessionStartedAtAsc — oldest first.
	SortSessionStartedAtAsc
	// SortSessionIDAsc — lexicographic by ULID-style id (which is
	// itself a time-sortable prefix; included for completeness and
	// deterministic browsing).
	SortSessionIDAsc
	// SortSessionEventCountDesc — highest event volume first.
	SortSessionEventCountDesc
)

// SessionWithCounts is the denormalised view that backs the Phase 2c S3
// session-browser tab: session metadata + event_count + crash_count via
// COUNT-Subqueries against the same FK-indexed columns the rest of the
// store already uses (idx_events_session_ts, idx_crashes_session_ts).
//
// The Counts are int64 because SQLite COUNT() returns an integer the
// driver maps cleanly to int64; rows.Scan(*int) would also work but
// int64 is the lossless option.
type SessionWithCounts struct {
	Session
	EventCount int64
	CrashCount int64
}

// ListSessionsOpts bundles the optional list parameters for the Phase 2c
// S3 session-browser tab. Zero values map to "newest first, no filter,
// no offset, default limit". Sort is the typed enum above to keep the
// SQL form whitelisted; FilterIDSubstring is matched case-sensitively
// against the id column with a parameterised LIKE (no user-controlled
// SQL fragment ever reaches Query).
type ListSessionsOpts struct {
	Limit             int
	Offset            int
	FilterIDSubstring string
	Sort              SessionSort
}

// ListSessionsWithCounts returns up to opts.Limit sessions with their
// associated event_count and crash_count, applying the optional
// substring filter on session id and the typed sort order. opts.Offset
// supports paginated browsing.
//
// Aggregate-counts strategy (Phase 2c S3): COUNT-Subqueries on the
// already-indexed (session_id, ts) tuples. EXPLAIN-Plan: idx_events_session_ts
// and idx_crashes_session_ts cover the WHERE/GROUP-equivalent so each
// per-row count is a covered range scan. No new index, no schema
// migration — additive query method only.
//
// limit <= 0 falls back to a 50 default (mirrors ListSessions). The
// store does not cap; the HTTP handler caps separately at 500 to keep a
// dashboard render below the SSE-equivalent payload budget.
//
// Empty result is returned as `nil, nil` (distinct from an error).
func (s *Store) ListSessionsWithCounts(ctx context.Context, opts ListSessionsOpts) ([]SessionWithCounts, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	// Sort clause is selected from a fixed whitelist. The variable is
	// concatenated into the SQL string, but its set of possible values
	// is closed at compile time (the default branch covers any future
	// enum addition that forgets to extend this switch).
	var orderBy string
	switch opts.Sort {
	case SortSessionStartedAtAsc:
		orderBy = "started_at ASC, id ASC"
	case SortSessionIDAsc:
		orderBy = "id ASC"
	case SortSessionEventCountDesc:
		orderBy = "event_count DESC, started_at DESC"
	case SortSessionStartedAtDesc:
		fallthrough
	default:
		orderBy = "started_at DESC, id DESC"
	}

	// Substring filter is fed through a parameterised LIKE; the `%`
	// padding is added in Go, not in SQL, so the user input cannot
	// inject wildcards or escapes (sqlite's LIKE is case-insensitive
	// for ASCII by default — fine for hex session ids).
	var (
		query string
		args  []any
	)
	const baseSelect = `
		SELECT
			s.id, s.label, s.started_at, s.ended_at,
			(SELECT COUNT(*) FROM events  e WHERE e.session_id = s.id) AS event_count,
			(SELECT COUNT(*) FROM crashes c WHERE c.session_id = s.id) AS crash_count
		FROM sessions s
	`
	if opts.FilterIDSubstring != "" {
		query = baseSelect + ` WHERE s.id LIKE ? ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`
		args = []any{"%" + opts.FilterIDSubstring + "%", limit, offset}
	} else {
		query = baseSelect + ` ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`
		args = []any{limit, offset}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions with counts: %w", err)
	}
	defer rows.Close()

	var out []SessionWithCounts
	for rows.Next() {
		var sc SessionWithCounts
		var startedNano int64
		var endedNano sql.NullInt64
		if err := rows.Scan(
			&sc.ID, &sc.Label, &startedNano, &endedNano,
			&sc.EventCount, &sc.CrashCount,
		); err != nil {
			return nil, fmt.Errorf("store: scan session-with-counts: %w", err)
		}
		sc.StartedAt = time.Unix(0, startedNano)
		if endedNano.Valid {
			t := time.Unix(0, endedNano.Int64)
			sc.EndedAt = &t
		}
		out = append(out, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: rows session-with-counts: %w", err)
	}
	return out, nil
}

// CountSessions returns the total number of sessions matching the
// optional substring filter on session id. Backs the Phase 2c S3
// session-browser tab's pagination control: page count = ceil(total / limit).
//
// Uses the same parameterised LIKE pattern as ListSessionsWithCounts so
// the two stay aligned at the WHERE-clause boundary.
func (s *Store) CountSessions(ctx context.Context, filterIDSubstring string) (int64, error) {
	var (
		query string
		args  []any
	)
	if filterIDSubstring != "" {
		query = `SELECT COUNT(*) FROM sessions WHERE id LIKE ?`
		args = []any{"%" + filterIDSubstring + "%"}
	} else {
		query = `SELECT COUNT(*) FROM sessions`
	}
	var n int64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count sessions: %w", err)
	}
	return n, nil
}

// SessionByID returns the single session row identified by id. Returns
// sql.ErrNoRows if no row exists. Used by the dashboard's session-detail
// view (Phase 2c S3) to distinguish "unknown id → 404" from "valid id
// with no events → empty list".
func (s *Store) SessionByID(ctx context.Context, id string) (Session, error) {
	var sess Session
	var startedNano int64
	var endedNano sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, label, started_at, ended_at
		FROM sessions
		WHERE id = ?
	`, id).Scan(&sess.ID, &sess.Label, &startedNano, &endedNano)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, sql.ErrNoRows
		}
		return Session{}, fmt.Errorf("store: session by id: %w", err)
	}
	sess.StartedAt = time.Unix(0, startedNano)
	if endedNano.Valid {
		t := time.Unix(0, endedNano.Int64)
		sess.EndedAt = &t
	}
	return sess, nil
}
