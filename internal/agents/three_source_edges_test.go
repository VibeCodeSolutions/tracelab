// three_source_edges_test.go — Phase 2d S5 cross-source edges check.
//
// Three ingest sources (sdk-hook, transcript, mcp-push) all push the
// SAME mailbox-edge tuple (from, to, edge_type, ts) and the schema's
// UNIQUE-tuple constraint coalesces them to ONE row. This is the
// edges-side mirror of TestAgentsIngestThreeSourceCoexistence (which
// covered tokens) — edges, unlike tokens, do not preserve per-source
// breakdown (the schema has no source column on agent_mailbox_edges).
// That is deliberate: cross-source agreement on an edge is the safety
// signal; cross-source disagreement on tokens is the forensics signal.
//
// Source: AUFTRAG #036 Deliverable 1 cross-source test (analog
// 2/3-Source coexistence). SDK-hook does not currently emit edges from
// the hook-script (S2-S5 scope decision — edges are S5 transcript
// + mcp-push primary, sdk-hook secondary by design), but the WIRE-LEVEL
// schema accepts edges from any source. This test asserts that
// behaviour so future hook-script work (Nicoletti domain) can add
// edge-emit without revisiting the wire contract.

package agents

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestAgentsIngestThreeSourceEdgesCollapseOnUniqueTuple(t *testing.T) {
	h, st := newTestHandler(t)

	const parentID = "p1234567890123456789abcdef"
	const childID = "c1234567890123456789abcdef"
	ts := time.Unix(1700000002, 0).UnixNano()

	// Seed both spawn rows (FK precondition for the edge insert).
	rr := postIngest(t, h, IngestPayload{
		Source: SourceSDKHook,
		Spawn: &SpawnPayload{
			ID: parentID, Skill: "belanna", StartedAt: ts, Project: "tracelab",
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("seed parent status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = postIngest(t, h, IngestPayload{
		Source: SourceSDKHook,
		Spawn: &SpawnPayload{
			ID: childID, ParentID: parentID, Skill: "ballard", StartedAt: ts, Project: "tracelab",
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("seed child status=%d body=%s", rr.Code, rr.Body.String())
	}

	edge := MailboxEdgePayload{
		FromSpawnID: parentID,
		ToSpawnID:   childID,
		EdgeType:    "spawn",
		TS:          ts,
	}

	// (1) SDK-hook pushes the edge.
	rr = postIngest(t, h, IngestPayload{
		Source:       SourceSDKHook,
		MailboxEdges: []MailboxEdgePayload{edge},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("sdk-hook edge status=%d body=%s", rr.Code, rr.Body.String())
	}

	// (2) Transcript-tail picks up the same edge from the JSONL.
	rr = postIngest(t, h, IngestPayload{
		Source:       SourceTranscript,
		MailboxEdges: []MailboxEdgePayload{edge},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("transcript edge status=%d body=%s", rr.Code, rr.Body.String())
	}

	// (3) MCP-push reports the same edge a third time.
	rr = postIngest(t, h, IngestPayload{
		Source:       SourceMCPPush,
		MailboxEdges: []MailboxEdgePayload{edge},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("mcp-push edge status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Schema assertion: exactly ONE edge row survives — the
	// UNIQUE (from, to, edge_type, ts) tuple collapses the second
	// and third writes via INSERT OR IGNORE.
	var edgeCount int
	if err := st.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM agent_mailbox_edges
		  WHERE from_spawn_id = ? AND to_spawn_id = ? AND edge_type = ? AND ts = ?`,
		parentID, childID, "spawn", ts,
	).Scan(&edgeCount); err != nil {
		t.Fatalf("query edges: %v", err)
	}
	if edgeCount != 1 {
		t.Errorf("agent_mailbox_edges rows for (parent→child/spawn/ts): want 1, got %d (3 writers must dedupe on UNIQUE tuple)",
			edgeCount)
	}
}
