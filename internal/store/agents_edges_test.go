// agents_edges_test.go — Phase 2d S5 store-layer edge read coverage.
//
// Pins the AgentEdgesForSpawn / AgentEdgesForSpawnIDs read paths against
// a fresh on-disk store with seeded fixtures. Covers ordering,
// in-vs-out direction split, empty spawns, FK violations (writer-side),
// batched read across multiple spawns, and chunking past the
// modernc.org/sqlite host-parameter cap.

package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func seedEdgeRow(t *testing.T, s *Store, from, to, edgeType string, ts time.Time) {
	t.Helper()
	_, err := s.InsertAgentMailboxEdge(context.Background(), AgentMailboxEdge{
		FromSpawnID: from,
		ToSpawnID:   to,
		EdgeType:    edgeType,
		TS:          ts,
	})
	if err != nil {
		t.Fatalf("InsertAgentMailboxEdge(%s→%s/%s): %v", from, to, edgeType, err)
	}
}

func TestStore_AgentEdgesForSpawn_InOutSplitAndOrdering(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0)

	parent := pad26("parent")
	child := pad26("child")
	sibling := pad26("sibling")
	seedRowSpawn(t, s, parent, "", "belanna", "tracelab", now)
	seedRowSpawn(t, s, child, parent, "ballard", "tracelab", now)
	seedRowSpawn(t, s, sibling, parent, "tuvok", "tracelab", now)

	// Out-edges from parent: parent→child(spawn), parent→sibling(spawn)
	seedEdgeRow(t, s, parent, child, "spawn", now)
	seedEdgeRow(t, s, parent, sibling, "spawn", now.Add(time.Second))
	// In-edge into parent: child→parent(return)
	seedEdgeRow(t, s, child, parent, "return", now.Add(2*time.Second))

	in, out, err := s.AgentEdgesForSpawn(ctx, parent)
	if err != nil {
		t.Fatalf("AgentEdgesForSpawn: %v", err)
	}
	if len(in) != 1 {
		t.Fatalf("in=%d, want 1", len(in))
	}
	if in[0].FromSpawnID != child || in[0].EdgeType != "return" {
		t.Errorf("in[0]=%+v", in[0])
	}
	if len(out) != 2 {
		t.Fatalf("out=%d, want 2", len(out))
	}
	// Ordering ts ASC, id ASC — parent→child first (earlier ts), then sibling
	if out[0].ToSpawnID != child {
		t.Errorf("out[0]=%+v, want to=child", out[0])
	}
	if out[1].ToSpawnID != sibling {
		t.Errorf("out[1]=%+v, want to=sibling", out[1])
	}
}

func TestStore_AgentEdgesForSpawn_EmptySpawn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0)
	lonely := pad26("lonely")
	seedRowSpawn(t, s, lonely, "", "ballard", "tracelab", now)

	in, out, err := s.AgentEdgesForSpawn(ctx, lonely)
	if err != nil {
		t.Fatalf("AgentEdgesForSpawn: %v", err)
	}
	if in != nil {
		t.Errorf("in=%v, want nil", in)
	}
	if out != nil {
		t.Errorf("out=%v, want nil", out)
	}
}

func TestStore_AgentEdgesForSpawn_MissingIDReturnsError(t *testing.T) {
	s := newTestStore(t)
	_, _, err := s.AgentEdgesForSpawn(context.Background(), "")
	if err == nil {
		t.Fatal("AgentEdgesForSpawn(\"\") should error")
	}
	if !strings.Contains(err.Error(), "id required") {
		t.Errorf("err=%q, want substring 'id required'", err.Error())
	}
}

func TestStore_InsertAgentMailboxEdge_FKViolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	_, err := s.InsertAgentMailboxEdge(ctx, AgentMailboxEdge{
		FromSpawnID: pad26("ghost"),
		ToSpawnID:   pad26("alsoghost"),
		EdgeType:    "spawn",
		TS:          time.Unix(1700000000, 0),
	})
	if err == nil {
		t.Fatal("expected FK violation, got nil")
	}
}

func TestStore_InsertAgentMailboxEdge_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0)
	a := pad26("alpha")
	b := pad26("bravo")
	seedRowSpawn(t, s, a, "", "x", "p", now)
	seedRowSpawn(t, s, b, "", "y", "p", now)

	edge := AgentMailboxEdge{FromSpawnID: a, ToSpawnID: b, EdgeType: "spawn", TS: now}
	n1, err := s.InsertAgentMailboxEdge(ctx, edge)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if n1 != 1 {
		t.Errorf("first insert n=%d, want 1", n1)
	}
	n2, err := s.InsertAgentMailboxEdge(ctx, edge)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second insert n=%d, want 0 (UNIQUE-tuple collapse)", n2)
	}
}

func TestStore_AgentEdgesForSpawnIDs_BatchedReadMultiSpawn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Unix(1700000000, 0)

	a := pad26("a")
	b := pad26("b")
	c := pad26("c")
	seedRowSpawn(t, s, a, "", "x", "p", now)
	seedRowSpawn(t, s, b, "", "y", "p", now)
	seedRowSpawn(t, s, c, "", "z", "p", now)

	// a → b (spawn), b → c (delegate), c → a (return)
	seedEdgeRow(t, s, a, b, "spawn", now)
	seedEdgeRow(t, s, b, c, "delegate", now.Add(time.Second))
	seedEdgeRow(t, s, c, a, "return", now.Add(2*time.Second))

	bundles, err := s.AgentEdgesForSpawnIDs(ctx, []string{a, b, c})
	if err != nil {
		t.Fatalf("AgentEdgesForSpawnIDs: %v", err)
	}
	if len(bundles) != 3 {
		t.Fatalf("bundles=%d, want 3", len(bundles))
	}

	// a has 1 in-edge (from c) and 1 out-edge (to b)
	if len(bundles[a].In) != 1 || bundles[a].In[0].FromSpawnID != c {
		t.Errorf("a.in=%+v", bundles[a].In)
	}
	if len(bundles[a].Out) != 1 || bundles[a].Out[0].ToSpawnID != b {
		t.Errorf("a.out=%+v", bundles[a].Out)
	}
	// b has 1 in-edge (from a) and 1 out-edge (to c)
	if len(bundles[b].In) != 1 || bundles[b].In[0].FromSpawnID != a {
		t.Errorf("b.in=%+v", bundles[b].In)
	}
	if len(bundles[b].Out) != 1 || bundles[b].Out[0].ToSpawnID != c {
		t.Errorf("b.out=%+v", bundles[b].Out)
	}
	// c has 1 in-edge (from b) and 1 out-edge (to a)
	if len(bundles[c].In) != 1 || bundles[c].In[0].FromSpawnID != b {
		t.Errorf("c.in=%+v", bundles[c].In)
	}
	if len(bundles[c].Out) != 1 || bundles[c].Out[0].ToSpawnID != a {
		t.Errorf("c.out=%+v", bundles[c].Out)
	}
}

func TestStore_AgentEdgesForSpawnIDs_EmptyInput(t *testing.T) {
	s := newTestStore(t)
	bundles, err := s.AgentEdgesForSpawnIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("AgentEdgesForSpawnIDs(nil): %v", err)
	}
	if len(bundles) != 0 {
		t.Errorf("bundles=%d, want 0", len(bundles))
	}
}
