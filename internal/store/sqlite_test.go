package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func TestOpenAndMigrate(t *testing.T) {
	s := newTestStore(t)

	var version int
	err := s.db.QueryRowContext(context.Background(),
		`SELECT MAX(version) FROM schema_migrations`).Scan(&version)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if version != 2 {
		t.Fatalf("want version 2, got %d", version)
	}

	// All four tables must exist.
	for _, tbl := range []string{"sessions", "events", "crashes", "screenshots"} {
		var name string
		err := s.db.QueryRowContext(context.Background(),
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", tbl, err)
		}
	}

	// Migration 0002: unique index on (session_id, fingerprint) must exist.
	var idxName string
	err = s.db.QueryRowContext(context.Background(),
		`SELECT name FROM sqlite_master WHERE type='index' AND name=?`,
		"idx_crashes_session_fp",
	).Scan(&idxName)
	if err != nil {
		t.Errorf("idx_crashes_session_fp missing: %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateSession(ctx, "smoke")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if id == "" {
		t.Fatal("empty session id")
	}

	events := []Event{
		{Source: "logcat", Level: "info", Msg: "boot complete", TS: time.Unix(1700000000, 0)},
		{Source: "logcat", Level: "warn", Msg: "low memory", TS: time.Unix(1700000001, 0),
			Meta: json.RawMessage(`{"free":1024}`)},
		{Source: "app", Level: "error", Msg: "kaboom", TS: time.Unix(1700000002, 0)},
	}
	if err := s.InsertEvents(ctx, id, events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	got, err := s.RecentEvents(ctx, id, 10)
	if err != nil {
		t.Fatalf("RecentEvents: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 events, got %d", len(got))
	}
	if got[0].Msg != "boot complete" || got[2].Msg != "kaboom" {
		t.Errorf("ordering wrong: %+v", got)
	}
	if string(got[1].Meta) != `{"free":1024}` {
		t.Errorf("meta roundtrip: got %q", string(got[1].Meta))
	}

	// Limit clamps newest.
	tail, err := s.RecentEvents(ctx, id, 2)
	if err != nil {
		t.Fatalf("RecentEvents limit: %v", err)
	}
	if len(tail) != 2 || tail[0].Msg != "low memory" || tail[1].Msg != "kaboom" {
		t.Errorf("limit-2 result wrong: %+v", tail)
	}

	if err := s.EndSession(ctx, id); err != nil {
		t.Fatalf("EndSession: %v", err)
	}

	sessions, err := s.ListSessions(ctx, 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(sessions))
	}
	if sessions[0].EndedAt == nil {
		t.Error("EndedAt should be set after EndSession")
	}
	if sessions[0].Label != "smoke" {
		t.Errorf("label: want smoke, got %q", sessions[0].Label)
	}
}

func TestIdempotentMigrations(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "idem.db")
	s1, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}

	s2, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	defer s2.Close()

	var count int
	if err := s2.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("schema_migrations count = %d, want 2 (idempotent)", count)
	}
}

func TestForeignKeyCascade(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateSession(ctx, "cascade")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.InsertEvents(ctx, id, []Event{
		{Source: "x", Level: "info", Msg: "a"},
		{Source: "x", Level: "info", Msg: "b"},
	}); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM events WHERE session_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 0 {
		t.Fatalf("events should cascade-delete, got %d remaining", n)
	}
}

func TestUpsertCrashFirstInsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateSession(ctx, "crash-1")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	ts := time.Unix(1700000000, 0)
	if err := s.UpsertCrash(ctx, id, ts, "fp-001-abc", "stack body\n at Foo.bar"); err != nil {
		t.Fatalf("UpsertCrash: %v", err)
	}

	crashes, err := s.CrashesBySession(ctx, id, 0)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(crashes) != 1 {
		t.Fatalf("want 1 crash row, got %d", len(crashes))
	}
	c := crashes[0]
	if c.Count != 1 {
		t.Errorf("count = %d, want 1", c.Count)
	}
	if c.Fingerprint != "fp-001-abc" {
		t.Errorf("fingerprint = %q, want fp-001-abc", c.Fingerprint)
	}
	if !c.TS.Equal(ts) {
		t.Errorf("ts = %v, want %v", c.TS, ts)
	}
}

func TestUpsertCrashDedup(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateSession(ctx, "crash-dedup")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	for i := 0; i < 3; i++ {
		ts := time.Unix(int64(1700000000+i), 0)
		if err := s.UpsertCrash(ctx, id, ts, "fp-dup", "trace v"+string(rune('0'+i))); err != nil {
			t.Fatalf("UpsertCrash #%d: %v", i, err)
		}
	}

	crashes, err := s.CrashesBySession(ctx, id, 0)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(crashes) != 1 {
		t.Fatalf("want 1 dedup row, got %d", len(crashes))
	}
	if crashes[0].Count != 3 {
		t.Errorf("count = %d, want 3", crashes[0].Count)
	}
	// TS should reflect the most recent occurrence.
	wantTS := time.Unix(1700000002, 0)
	if !crashes[0].TS.Equal(wantTS) {
		t.Errorf("ts = %v, want %v", crashes[0].TS, wantTS)
	}
	// Stacktrace stays at the first-inserted body (we don't overwrite).
	if crashes[0].Stacktrace != "trace v0" {
		t.Errorf("stacktrace = %q, want trace v0 (first insert wins)", crashes[0].Stacktrace)
	}
}

func TestUpsertCrashDistinctFingerprints(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateSession(ctx, "crash-distinct")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	ts := time.Unix(1700000000, 0)
	if err := s.UpsertCrash(ctx, id, ts, "fp-a", "trace A"); err != nil {
		t.Fatalf("UpsertCrash A: %v", err)
	}
	if err := s.UpsertCrash(ctx, id, ts, "fp-b", "trace B"); err != nil {
		t.Fatalf("UpsertCrash B: %v", err)
	}
	crashes, err := s.CrashesBySession(ctx, id, 0)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(crashes) != 2 {
		t.Fatalf("want 2 distinct rows, got %d", len(crashes))
	}
}

func TestUpsertCrashRejectsUnknownSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	err := s.UpsertCrash(ctx, "ghost-session", time.Now(), "fp", "stack")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows for unknown session, got %v", err)
	}
}

func TestUpsertCrashRejectsEmptyFingerprint(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, err := s.CreateSession(ctx, "crash-empty")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.UpsertCrash(ctx, id, time.Now(), "", "stack"); err == nil {
		t.Fatal("want error for empty fingerprint, got nil")
	}
}

func TestInsertEventsRejectsUnknownSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	err := s.InsertEvents(ctx, "ghost-session", []Event{
		{Source: "x", Level: "info", Msg: "orphan"},
	})
	if err == nil {
		t.Fatal("expected FK violation, got nil")
	}
}

// TestEventsSinceCursorAdvances seeds five events for one session, then
// walks the cursor forward in batches of two — the canonical Phase-2b S4
// polling loop (ADR-008). Verifies (a) strict `id > since_seq` semantics
// (cursor never re-reads), (b) ascending id-order in each page, (c)
// next_since_seq monotonically tracks the last returned id.
func TestEventsSinceCursorAdvances(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, err := s.CreateSession(ctx, "since-walk")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	events := make([]Event, 5)
	for i := range events {
		events[i] = Event{
			TS:     time.Unix(int64(1700000000+i), 0),
			Source: "test", Level: "info", Msg: fmt.Sprintf("evt-%d", i),
		}
	}
	if err := s.InsertEvents(ctx, id, events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	var cursor int64
	var collected []string
	for round := 0; round < 4; round++ { // bounded to avoid runaway
		batch, err := s.EventsSince(ctx, id, cursor, 2)
		if err != nil {
			t.Fatalf("EventsSince round %d: %v", round, err)
		}
		if len(batch) == 0 {
			break
		}
		// Ascending id order within a page.
		for i := 1; i < len(batch); i++ {
			if batch[i].ID <= batch[i-1].ID {
				t.Errorf("page %d not ascending: %d <= %d", round, batch[i].ID, batch[i-1].ID)
			}
		}
		// Cursor must strictly advance — no re-read.
		if batch[0].ID <= cursor {
			t.Errorf("page %d first id %d <= cursor %d (strict-greater-than violated)", round, batch[0].ID, cursor)
		}
		for _, e := range batch {
			collected = append(collected, e.Msg)
		}
		cursor = batch[len(batch)-1].ID
	}
	want := []string{"evt-0", "evt-1", "evt-2", "evt-3", "evt-4"}
	if len(collected) != len(want) {
		t.Fatalf("collected %d events, want %d: %v", len(collected), len(want), collected)
	}
	for i := range want {
		if collected[i] != want[i] {
			t.Errorf("collected[%d] = %q, want %q", i, collected[i], want[i])
		}
	}
}

// TestEventsSinceEmptyResult covers two empty-result cases that must
// both succeed cleanly with a nil-or-zero-length slice and a nil error:
//
//   - session ID does not exist (cursor read is not a session probe).
//   - session exists but no events at-or-after the cursor.
//
// These are the two paths that drive the "stable on empty" property in
// the HTTP handler: next_since_seq stays at the caller's input when
// nothing is returned, so a polling loop never spins on a stale cursor.
func TestEventsSinceEmptyResult(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Case 1: unknown session.
	got, err := s.EventsSince(ctx, "ghost-session", 0, 100)
	if err != nil {
		t.Fatalf("unknown session: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unknown session: want empty, got %d events", len(got))
	}

	// Case 2: known session, cursor past the highest event id.
	id, err := s.CreateSession(ctx, "since-empty")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.InsertEvents(ctx, id, []Event{{Source: "x", Level: "info", Msg: "only"}}); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}
	// Establish the current max id, then read with a cursor at that max.
	all, err := s.EventsSince(ctx, id, 0, 10)
	if err != nil {
		t.Fatalf("warmup EventsSince: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("warmup: want 1 event, got %d", len(all))
	}
	maxID := all[0].ID
	tail, err := s.EventsSince(ctx, id, maxID, 10)
	if err != nil {
		t.Fatalf("tail EventsSince: %v", err)
	}
	if len(tail) != 0 {
		t.Errorf("cursor at max: want empty, got %d events", len(tail))
	}
}

// TestEventsSinceLimitDefault asserts that limit <= 0 falls back to the
// 500-row default (matches the HTTP handler's default, see ADR-008).
// We seed three events and pass limit=0 — all three must come back.
func TestEventsSinceLimitDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, err := s.CreateSession(ctx, "since-default")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	events := []Event{
		{Source: "a", Level: "info", Msg: "one"},
		{Source: "a", Level: "info", Msg: "two"},
		{Source: "a", Level: "info", Msg: "three"},
	}
	if err := s.InsertEvents(ctx, id, events); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}
	got, err := s.EventsSince(ctx, id, 0, 0)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("limit=0 default: want 3, got %d", len(got))
	}
}

// TestEventsSinceCrossSessionIsolation puts events in two sessions and
// verifies the cursor walks only one session's rows — even though
// `events.id` is globally monotonic (per ADR-008 Decision 1), the
// per-session filter must not leak rows from another session.
func TestEventsSinceCrossSessionIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, err := s.CreateSession(ctx, "session-a")
	if err != nil {
		t.Fatalf("CreateSession A: %v", err)
	}
	b, err := s.CreateSession(ctx, "session-b")
	if err != nil {
		t.Fatalf("CreateSession B: %v", err)
	}
	// Interleave inserts so A and B share id-neighbourhood.
	if err := s.InsertEvents(ctx, a, []Event{{Source: "a", Level: "info", Msg: "a1"}}); err != nil {
		t.Fatalf("Insert a1: %v", err)
	}
	if err := s.InsertEvents(ctx, b, []Event{{Source: "b", Level: "info", Msg: "b1"}}); err != nil {
		t.Fatalf("Insert b1: %v", err)
	}
	if err := s.InsertEvents(ctx, a, []Event{{Source: "a", Level: "info", Msg: "a2"}}); err != nil {
		t.Fatalf("Insert a2: %v", err)
	}
	if err := s.InsertEvents(ctx, b, []Event{{Source: "b", Level: "info", Msg: "b2"}}); err != nil {
		t.Fatalf("Insert b2: %v", err)
	}

	got, err := s.EventsSince(ctx, a, 0, 100)
	if err != nil {
		t.Fatalf("EventsSince A: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("session A: want 2 events, got %d", len(got))
	}
	for _, e := range got {
		if e.SessionID != a {
			t.Errorf("session A leak: got event with session_id=%q", e.SessionID)
		}
		if e.Source != "a" {
			t.Errorf("session A leak by source: %+v", e)
		}
	}
}

// TestCrashesBySessionLimitDefault asserts that limit <= 0 falls back to
// the 500-row default (mirrors EventsSince and ADR-009 Decision 1). We
// seed three crashes and pass limit=0 — all three must come back.
func TestCrashesBySessionLimitDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, err := s.CreateSession(ctx, "crash-limit-default")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	for i := 0; i < 3; i++ {
		fp := fmt.Sprintf("fp-default-%d", i)
		ts := time.Unix(int64(1700000000+i), 0)
		if err := s.UpsertCrash(ctx, id, ts, fp, "stack "+fp); err != nil {
			t.Fatalf("UpsertCrash %d: %v", i, err)
		}
	}
	got, err := s.CrashesBySession(ctx, id, 0)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("limit=0 default: want 3, got %d", len(got))
	}
}

// TestCrashesBySessionLimitClamps asserts that a small limit caps the
// returned slice and that the newest crashes come first (ORDER BY
// ts DESC, id DESC). Seeds five distinct crashes with strictly
// increasing ts; limit=2 must return the two newest.
func TestCrashesBySessionLimitClamps(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, err := s.CreateSession(ctx, "crash-limit-clamp")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	for i := 0; i < 5; i++ {
		fp := fmt.Sprintf("fp-clamp-%d", i)
		ts := time.Unix(int64(1700000000+i), 0)
		if err := s.UpsertCrash(ctx, id, ts, fp, "stack "+fp); err != nil {
			t.Fatalf("UpsertCrash %d: %v", i, err)
		}
	}
	got, err := s.CrashesBySession(ctx, id, 2)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit=2: want 2 rows, got %d", len(got))
	}
	// Newest first: fp-clamp-4 (ts=...004), then fp-clamp-3.
	if got[0].Fingerprint != "fp-clamp-4" {
		t.Errorf("first row fp = %q, want fp-clamp-4", got[0].Fingerprint)
	}
	if got[1].Fingerprint != "fp-clamp-3" {
		t.Errorf("second row fp = %q, want fp-clamp-3", got[1].Fingerprint)
	}
}

// TestCrashesBySessionEmptyResult asserts unknown-session and known-but-
// crash-free session both return an empty slice with a nil error. Mirrors
// the ADR-009 statement that /crashes is a list-read, not a session
// existence probe.
func TestCrashesBySessionEmptyResult(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Unknown session.
	got, err := s.CrashesBySession(ctx, "ghost-session", 10)
	if err != nil {
		t.Fatalf("unknown session: err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unknown session: want empty, got %d rows", len(got))
	}

	// Known but crash-free session.
	id, err := s.CreateSession(ctx, "crash-free")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err = s.CrashesBySession(ctx, id, 10)
	if err != nil {
		t.Fatalf("known empty session: err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("known empty session: want empty, got %d rows", len(got))
	}
}

// TestCrashesBySessionCrossSessionIsolation puts crashes in two sessions
// and verifies that the query returns only the requested session's
// rows. Mirror of the EventsSince isolation test.
func TestCrashesBySessionCrossSessionIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a, err := s.CreateSession(ctx, "crash-iso-a")
	if err != nil {
		t.Fatalf("CreateSession A: %v", err)
	}
	b, err := s.CreateSession(ctx, "crash-iso-b")
	if err != nil {
		t.Fatalf("CreateSession B: %v", err)
	}
	ts := time.Unix(1700000000, 0)
	if err := s.UpsertCrash(ctx, a, ts, "fp-a-1", "stack a1"); err != nil {
		t.Fatalf("UpsertCrash a1: %v", err)
	}
	if err := s.UpsertCrash(ctx, b, ts, "fp-b-1", "stack b1"); err != nil {
		t.Fatalf("UpsertCrash b1: %v", err)
	}
	if err := s.UpsertCrash(ctx, a, ts.Add(time.Second), "fp-a-2", "stack a2"); err != nil {
		t.Fatalf("UpsertCrash a2: %v", err)
	}

	got, err := s.CrashesBySession(ctx, a, 100)
	if err != nil {
		t.Fatalf("CrashesBySession A: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("session A: want 2 crashes, got %d", len(got))
	}
	for _, c := range got {
		if c.SessionID != a {
			t.Errorf("session A leak: got crash with session_id=%q", c.SessionID)
		}
	}
}
