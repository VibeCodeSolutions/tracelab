package store

import (
	"context"
	"encoding/json"
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
