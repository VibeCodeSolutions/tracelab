// agents_event_refs_test.go — Phase 2d S5-Tail store-layer coverage for
// agent_event_refs (ADR-014 Accepted, Option B).
//
// Pins the InsertAgentEventRef / AgentEventRefsForSpawn read paths
// against a fresh on-disk store with seeded fixtures. Covers
// happy-path insert + read, ordering, idempotency via the
// UNIQUE (spawn_id, event_id, ref_type, ts) tuple, FK violations
// (missing spawn / missing event), and the empty-spawn case.

package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

// seedEventForRefs seeds one session + one event and returns the
// AUTOINCREMENT events.id so the FK from agent_event_refs.event_id has
// a real target. The session id is derived from the test name to keep
// fixtures isolated; a single session can host multiple events.
func seedEventForRefs(t *testing.T, s *Store, sessionLabel, source, level, msg string) int64 {
	t.Helper()
	ctx := context.Background()
	sessionID, err := s.CreateSession(ctx, sessionLabel)
	if err != nil {
		t.Fatalf("CreateSession(%s): %v", sessionLabel, err)
	}
	ev := Event{
		SessionID: sessionID,
		TS:        time.Unix(1700000000, 0),
		Source:    source,
		Level:     level,
		Msg:       msg,
	}
	if err := s.InsertEvents(ctx, sessionID, []Event{ev}); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}
	// Recover the AUTOINCREMENT id from the just-inserted row.
	var id int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM events WHERE session_id = ? ORDER BY id DESC LIMIT 1`,
		sessionID,
	).Scan(&id); err != nil {
		t.Fatalf("query event id: %v", err)
	}
	return id
}

func seedEventRefRow(t *testing.T, s *Store, spawnID string, eventID int64, refType string, ts time.Time) int64 {
	t.Helper()
	n, err := s.InsertAgentEventRef(context.Background(), AgentEventRef{
		SpawnID: spawnID,
		EventID: eventID,
		RefType: refType,
		TS:      ts,
	})
	if err != nil {
		t.Fatalf("InsertAgentEventRef(%s→%d/%s): %v", spawnID, eventID, refType, err)
	}
	return n
}

func TestStore_InsertAgentEventRef_HappyPath(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)

	spawnID := pad26("ref-happy")
	seedRowSpawn(t, s, spawnID, "", "ballard", "tracelab", now)
	eventID := seedEventForRefs(t, s, "happy", "app", "info", "first")

	n := seedEventRefRow(t, s, spawnID, eventID, "observed", now)
	if n != 1 {
		t.Errorf("first insert rows-affected=%d, want 1", n)
	}
}

func TestStore_AgentEventRefsForSpawn_OrderingAndRefTypes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0)

	spawnID := pad26("ref-order")
	seedRowSpawn(t, s, spawnID, "", "ballard", "tracelab", now)
	e1 := seedEventForRefs(t, s, "order-1", "app", "info", "e1")
	e2 := seedEventForRefs(t, s, "order-2", "app", "warn", "e2")
	e3 := seedEventForRefs(t, s, "order-3", "app", "error", "e3")

	// Insert in mixed order to exercise the ts ASC, id ASC ordering.
	seedEventRefRow(t, s, spawnID, e2, "context", now.Add(2*time.Second))
	seedEventRefRow(t, s, spawnID, e1, "observed", now)
	seedEventRefRow(t, s, spawnID, e3, "caused-by", now.Add(time.Second))

	rows, err := s.AgentEventRefsForSpawn(ctx, spawnID)
	if err != nil {
		t.Fatalf("AgentEventRefsForSpawn: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows=%d, want 3", len(rows))
	}
	// Expect ts ASC: e1 (now), e3 (now+1s), e2 (now+2s).
	wantOrder := []struct {
		eventID int64
		refType string
	}{
		{e1, "observed"},
		{e3, "caused-by"},
		{e2, "context"},
	}
	for i, want := range wantOrder {
		if rows[i].EventID != want.eventID {
			t.Errorf("rows[%d].EventID=%d, want %d", i, rows[i].EventID, want.eventID)
		}
		if rows[i].RefType != want.refType {
			t.Errorf("rows[%d].RefType=%q, want %q", i, rows[i].RefType, want.refType)
		}
		if rows[i].SpawnID != spawnID {
			t.Errorf("rows[%d].SpawnID=%q, want %q", i, rows[i].SpawnID, spawnID)
		}
		if rows[i].ID <= 0 {
			t.Errorf("rows[%d].ID=%d, want > 0 (AUTOINCREMENT)", i, rows[i].ID)
		}
	}
}

func TestStore_AgentEventRefsForSpawn_EmptySpawn(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)

	spawnID := pad26("ref-empty")
	seedRowSpawn(t, s, spawnID, "", "tuvok", "tracelab", now)

	rows, err := s.AgentEventRefsForSpawn(context.Background(), spawnID)
	if err != nil {
		t.Fatalf("AgentEventRefsForSpawn: %v", err)
	}
	if rows != nil {
		t.Errorf("rows=%v, want nil (empty result is nil convention)", rows)
	}
}

func TestStore_InsertAgentEventRef_IdempotencyOnUniqueTuple(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	ctx := context.Background()

	spawnID := pad26("ref-idem")
	seedRowSpawn(t, s, spawnID, "", "ballard", "tracelab", now)
	eventID := seedEventForRefs(t, s, "idem", "app", "info", "msg")

	// First insert inserts; second insert with identical tuple is a no-op.
	n1 := seedEventRefRow(t, s, spawnID, eventID, "observed", now)
	n2 := seedEventRefRow(t, s, spawnID, eventID, "observed", now)
	if n1 != 1 {
		t.Errorf("first rows-affected=%d, want 1", n1)
	}
	if n2 != 0 {
		t.Errorf("repeat rows-affected=%d, want 0 (UNIQUE tuple collapses via INSERT OR IGNORE)", n2)
	}

	// Differing ref_type produces a distinct row (different UNIQUE tuple).
	n3 := seedEventRefRow(t, s, spawnID, eventID, "context", now)
	if n3 != 1 {
		t.Errorf("different ref_type rows-affected=%d, want 1", n3)
	}

	// Final count: 2 rows survived (observed + context).
	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_event_refs WHERE spawn_id = ?`,
		spawnID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("surviving rows=%d, want 2 (observed + context)", count)
	}
}

func TestStore_InsertAgentEventRef_MissingFieldsError(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	ctx := context.Background()

	cases := []struct {
		name string
		ref  AgentEventRef
		want string
	}{
		{"empty spawn_id", AgentEventRef{EventID: 1, RefType: "observed", TS: now}, "spawn_id required"},
		{"zero event_id", AgentEventRef{SpawnID: "x", RefType: "observed", TS: now}, "event_id required"},
		{"negative event_id", AgentEventRef{SpawnID: "x", EventID: -1, RefType: "observed", TS: now}, "event_id required"},
		{"empty ref_type", AgentEventRef{SpawnID: "x", EventID: 1, TS: now}, "ref_type required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.InsertAgentEventRef(ctx, tc.ref)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err=%q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestStore_InsertAgentEventRef_FKViolations(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	ctx := context.Background()

	spawnID := pad26("ref-fk")
	seedRowSpawn(t, s, spawnID, "", "ballard", "tracelab", now)
	eventID := seedEventForRefs(t, s, "fk", "app", "info", "e1")

	// Bogus spawn_id → FK error on spawn_id side.
	_, err := s.InsertAgentEventRef(ctx, AgentEventRef{
		SpawnID: pad26("nonexistent"),
		EventID: eventID,
		RefType: "observed",
		TS:      now,
	})
	if err == nil {
		t.Fatal("want FK error for missing spawn, got nil")
	}

	// Bogus event_id → FK error on event_id side.
	_, err = s.InsertAgentEventRef(ctx, AgentEventRef{
		SpawnID: spawnID,
		EventID: 99999999,
		RefType: "observed",
		TS:      now,
	})
	if err == nil {
		t.Fatal("want FK error for missing event, got nil")
	}

	// Bogus ref_type: INSERT OR IGNORE silently drops the row on
	// CHECK-constraint violation (this is documented SQLite behaviour —
	// IGNORE applies to CHECK and UNIQUE, but not to FK). The handler
	// layer pre-validates ref_type via validEventRefTypes so the wire
	// path 400s before we reach the store; this assertion pins the
	// defence-in-depth posture at the schema layer.
	n, err := s.InsertAgentEventRef(ctx, AgentEventRef{
		SpawnID: spawnID,
		EventID: eventID,
		RefType: "bogus-ref-type",
		TS:      now,
	})
	if err != nil {
		t.Fatalf("INSERT OR IGNORE should not error on CHECK violation: %v", err)
	}
	if n != 0 {
		t.Errorf("bogus ref_type rows-affected=%d, want 0 (CHECK silently dropped by IGNORE)", n)
	}
	// Confirm no row landed.
	var bogusCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_event_refs WHERE ref_type = ?`, "bogus-ref-type",
	).Scan(&bogusCount); err != nil {
		t.Fatalf("count bogus: %v", err)
	}
	if bogusCount != 0 {
		t.Errorf("bogus ref_type rows in DB=%d, want 0", bogusCount)
	}
}

func TestStore_AgentEventRefsForSpawn_MissingIDReturnsError(t *testing.T) {
	s := newTestStore(t)
	_, err := s.AgentEventRefsForSpawn(context.Background(), "")
	if err == nil {
		t.Fatal("AgentEventRefsForSpawn(\"\") should error")
	}
	if !strings.Contains(err.Error(), "id required") {
		t.Errorf("err=%q, want substring 'id required'", err.Error())
	}
}
