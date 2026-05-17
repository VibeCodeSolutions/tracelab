// agents_read_test.go — Phase 2d S4 store-layer read coverage.
//
// Pins the read-query SQL forms against a fresh on-disk store with
// seeded fixtures. Covers ordering, filters, BFS tree-walk semantics,
// per-source token preservation, and NULL handling on LerneffektMD.

package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// pad26 mirrors the writer-supplied 26-char ULID-shape that the
// production ingest layer uses (see internal/agents/transcript.padTo26).
func pad26(prefix string) string {
	const n = 26
	if len(prefix) >= n {
		return prefix[:n]
	}
	out := []byte(prefix)
	for len(out) < n {
		out = append(out, 'a')
	}
	return string(out)
}

func seedRowSpawn(t *testing.T, s *Store, id, parent, skill, project string, started time.Time) {
	t.Helper()
	sp := AgentSpawn{ID: id, Skill: skill, Project: project, StartedAt: started}
	if parent != "" {
		sp.ParentID = parent
	}
	if _, err := s.InsertAgentSpawn(context.Background(), sp); err != nil {
		t.Fatalf("InsertAgentSpawn(%s): %v", id, err)
	}
}

func TestStore_ListAgentSpawns_OrderingAndPaging(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	seedRowSpawn(t, s, pad26("first"), "", "ballard", "tracelab", now)
	seedRowSpawn(t, s, pad26("second"), "", "tuvok", "tracelab", now.Add(time.Second))
	seedRowSpawn(t, s, pad26("third"), "", "barclay", "nexus", now.Add(2*time.Second))

	rows, err := s.ListAgentSpawns(context.Background(), ListAgentSpawnsOpts{Limit: 2})
	if err != nil {
		t.Fatalf("ListAgentSpawns: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len=%d, want 2", len(rows))
	}
	// Newest first
	if rows[0].ID != pad26("third") {
		t.Errorf("rows[0]=%q, want third", rows[0].ID)
	}
	if rows[1].ID != pad26("second") {
		t.Errorf("rows[1]=%q, want second", rows[1].ID)
	}

	// Offset 1 with limit 2 → second + first
	rows2, err := s.ListAgentSpawns(context.Background(), ListAgentSpawnsOpts{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListAgentSpawns offset: %v", err)
	}
	if rows2[0].ID != pad26("second") || rows2[1].ID != pad26("first") {
		t.Errorf("offset paging wrong: %+v", rows2)
	}
}

func TestStore_ListAgentSpawns_ProjectFilter(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	seedRowSpawn(t, s, pad26("t1"), "", "ballard", "tracelab", now)
	seedRowSpawn(t, s, pad26("t2"), "", "tuvok", "tracelab", now.Add(time.Second))
	seedRowSpawn(t, s, pad26("n1"), "", "barclay", "nexus", now.Add(2*time.Second))

	rows, err := s.ListAgentSpawns(context.Background(), ListAgentSpawnsOpts{FilterProject: "tracelab"})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("filter len=%d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Project != "tracelab" {
			t.Errorf("filter leak: %+v", r)
		}
	}
	total, err := s.CountAgentSpawns(context.Background(), "tracelab", "")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total != 2 {
		t.Errorf("CountAgentSpawns(tracelab)=%d, want 2", total)
	}
}

func TestStore_AgentSpawnByID(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	id := pad26("idtest")
	seedRowSpawn(t, s, id, "", "ballard", "tracelab", now)

	got, err := s.AgentSpawnByID(context.Background(), id)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.ID != id || got.Skill != "ballard" {
		t.Errorf("got=%+v", got)
	}

	_, err = s.AgentSpawnByID(context.Background(), pad26("nope"))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown id err=%v, want sql.ErrNoRows", err)
	}
}

func TestStore_ListAgentSpawnTree_BFS(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	root := pad26("root")
	c1 := pad26("childA")
	c2 := pad26("childB")
	gc := pad26("grandc")

	seedRowSpawn(t, s, root, "", "belanna", "tracelab", now)
	seedRowSpawn(t, s, c1, root, "ballard", "tracelab", now.Add(time.Second))
	seedRowSpawn(t, s, c2, root, "tuvok", "tracelab", now.Add(2*time.Second))
	seedRowSpawn(t, s, gc, c1, "harren", "tracelab", now.Add(3*time.Second))
	// A sibling spawn in another project must NOT leak in.
	seedRowSpawn(t, s, pad26("sib"), "", "barclay", "nexus", now)

	rows, err := s.ListAgentSpawnTree(context.Background(), root)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("tree len=%d, want 4", len(rows))
	}
	// BFS: root, c1, c2, gc (siblings ordered by started_at ASC)
	want := []string{root, c1, c2, gc}
	for i, r := range rows {
		if r.ID != want[i] {
			t.Errorf("rows[%d]=%q, want %q", i, r.ID, want[i])
		}
	}
}

func TestStore_AgentTokensBySpawn_PerSourcePreserved(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	spawn := pad26("toks")
	seedRowSpawn(t, s, spawn, "", "ballard", "tracelab", now)

	for i, src := range []string{"sdk-hook", "transcript", "mcp-push"} {
		if _, err := s.InsertAgentTokens(context.Background(), AgentTokenUsage{
			SpawnID:      spawn,
			InputTokens:  int64(100 * (i + 1)),
			OutputTokens: int64(50 * (i + 1)),
			TS:           now.Add(time.Duration(i) * time.Millisecond),
			Source:       src,
		}); err != nil {
			t.Fatalf("insert %s: %v", src, err)
		}
	}

	rows, err := s.AgentTokensBySpawn(context.Background(), spawn)
	if err != nil {
		t.Fatalf("AgentTokensBySpawn: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows len=%d, want 3", len(rows))
	}
	// ts ASC ordering preserves the insert order.
	want := []string{"sdk-hook", "transcript", "mcp-push"}
	for i, r := range rows {
		if r.Source != want[i] {
			t.Errorf("rows[%d].Source=%q, want %q", i, r.Source, want[i])
		}
	}
}

func TestStore_AgentVerdictsBySpawn_LerneffektNULLAndPresent(t *testing.T) {
	s := newTestStore(t)
	now := time.Unix(1700000000, 0)
	spawn := pad26("vts")
	seedRowSpawn(t, s, spawn, "", "tuvok", "tracelab", now)

	// First verdict: no lerneffekt (NULL on the wire)
	if _, err := s.InsertAgentVerdict(context.Background(), AgentVerdict{
		SpawnID: spawn, Verdict: "auflagen", TS: now,
	}); err != nil {
		t.Fatalf("v1: %v", err)
	}
	// Second verdict: with lerneffekt
	if _, err := s.InsertAgentVerdict(context.Background(), AgentVerdict{
		SpawnID: spawn, Verdict: "freigabe", LerneffektMD: "fixed", TS: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("v2: %v", err)
	}

	rows, err := s.AgentVerdictsBySpawn(context.Background(), spawn)
	if err != nil {
		t.Fatalf("AgentVerdictsBySpawn: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(rows))
	}
	if rows[0].LerneffektMD != "" {
		t.Errorf("rows[0].LerneffektMD=%q, want empty for NULL", rows[0].LerneffektMD)
	}
	if rows[1].LerneffektMD != "fixed" {
		t.Errorf("rows[1].LerneffektMD=%q, want 'fixed'", rows[1].LerneffektMD)
	}
}
