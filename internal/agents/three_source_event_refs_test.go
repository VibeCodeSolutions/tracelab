// three_source_event_refs_test.go — Phase 2d S5-Tail cross-source
// event-refs check (ADR-014 Accepted, Option B).
//
// Three ingest sources (sdk-hook, transcript, mcp-push) all push the
// SAME agent_event_refs tuple (spawn_id, event_id, ref_type, ts) and
// the schema's UNIQUE-tuple constraint coalesces them to ONE row. This
// is the event-refs-side mirror of:
//
//   - TestAgentsIngestThreeSourceCoexistence (tokens)
//   - TestAgentsIngestThreeSourceEdgesCollapseOnUniqueTuple (edges)
//
// Like edges, event_refs do NOT preserve per-source breakdown (the
// schema has no source column on agent_event_refs). Cross-source
// agreement on a reference is the safety signal; cross-source
// disagreement on tokens is the forensics signal — same posture as
// mailbox-edges (ADR-013 §Consequences, ADR-014 Decision).

package agents

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

func TestAgentsIngestThreeSourceEventRefsCollapseOnUniqueTuple(t *testing.T) {
	h, st := newTestHandler(t)
	ctx := context.Background()

	const spawnID = "r1234567890123456789abcdef"
	ts := time.Unix(1700000003, 0).UnixNano()

	// Seed the spawn (FK precondition for event_refs.spawn_id).
	rr := postIngest(t, h, IngestPayload{
		Source: SourceSDKHook,
		Spawn: &SpawnPayload{
			ID: spawnID, Skill: "ballard", StartedAt: ts, Project: "tracelab",
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("seed spawn status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Seed a session + event (FK precondition for event_refs.event_id).
	// The agents.Handler does not own the events surface, so we go
	// through the store directly — same posture seedEventForRefs uses
	// in the store-level tests.
	sessionID, err := st.CreateSession(ctx, "three-source-event-refs")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := st.InsertEvents(ctx, sessionID, []store.Event{{
		SessionID: sessionID,
		TS:        time.Unix(0, ts),
		Source:    "app",
		Level:     "warn",
		Msg:       "tripwire-fired",
	}}); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}
	var eventID int64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT id FROM events WHERE session_id = ? ORDER BY id DESC LIMIT 1`,
		sessionID,
	).Scan(&eventID); err != nil {
		t.Fatalf("query event id: %v", err)
	}

	ref := EventRefPayload{
		SpawnID: spawnID,
		EventID: eventID,
		RefType: "caused-by",
		TS:      ts,
	}

	// (1) SDK-hook reports the cross-domain reference.
	rr = postIngest(t, h, IngestPayload{
		Source:    SourceSDKHook,
		EventRefs: []EventRefPayload{ref},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("sdk-hook event_ref status=%d body=%s", rr.Code, rr.Body.String())
	}

	// (2) Transcript-tail reports the same reference. (The transcript
	// extractor doesn't actually emit event_refs in S5-Tail — it is a
	// Phase-3 bookmark per the worker brief. But the wire path accepts
	// transcript-sourced refs today, so this assertion pins the
	// behaviour for future hook-script work.)
	rr = postIngest(t, h, IngestPayload{
		Source:    SourceTranscript,
		EventRefs: []EventRefPayload{ref},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("transcript event_ref status=%d body=%s", rr.Code, rr.Body.String())
	}

	// (3) MCP-push reports the same reference a third time.
	rr = postIngest(t, h, IngestPayload{
		Source:    SourceMCPPush,
		EventRefs: []EventRefPayload{ref},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("mcp-push event_ref status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Schema assertion: exactly ONE row survives — the
	// UNIQUE (spawn_id, event_id, ref_type, ts) tuple collapses the
	// second and third writes via INSERT OR IGNORE.
	var refCount int
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_event_refs
		  WHERE spawn_id = ? AND event_id = ? AND ref_type = ? AND ts = ?`,
		spawnID, eventID, "caused-by", ts,
	).Scan(&refCount); err != nil {
		t.Fatalf("query event_refs: %v", err)
	}
	if refCount != 1 {
		t.Errorf("agent_event_refs rows for (spawn/event/caused-by/ts): want 1, got %d (3 writers must dedupe on UNIQUE tuple)",
			refCount)
	}
}
