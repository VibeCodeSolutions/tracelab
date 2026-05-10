package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	if version != 1 {
		t.Fatalf("want version 1, got %d", version)
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
	if count != 1 {
		t.Fatalf("schema_migrations count = %d, want 1 (idempotent)", count)
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

	crashes, err := s.CrashesBySession(ctx, id)
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

	crashes, err := s.CrashesBySession(ctx, id)
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
	crashes, err := s.CrashesBySession(ctx, id)
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
