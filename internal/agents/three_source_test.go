package agents

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestAgentsIngestThreeSourceCoexistence is the Phase-2d S3
// cross-verification: all three ingest sources (sdk-hook, transcript,
// mcp-push) push the SAME spawn at the SAME ts and produce three
// distinct rows in agent_tokens because source is part of the UNIQUE
// tuple. Same spawn_id collapses to one agent_spawns row because PK
// is just (id).
//
// This locks in ADR-013 §Consequences §Per-source-forensic-breakdown
// for the full 3-way live, closing the multi-ingest coverage loop
// opened in S1 (sdk-hook only) and S2 (sdk-hook + transcript).
//
// Source: AUFTRAG #034 Deliverable 4 (3-Source-Cross-Verifikation,
// PFLICHT). Schema-level test against a real store + handler —
// re-running this test in CI exercises the actual UNIQUE-tuple
// behaviour, not a mock.
func TestAgentsIngestThreeSourceCoexistence(t *testing.T) {
	h, st := newTestHandler(t)

	const spawnID = "01234567890123456789abcdef"
	ts := time.Unix(1700000001, 0).UnixNano()

	// (1) SDK-hook pushes the spawn + token counts (typical Phase-2d
	// S1 lifecycle event).
	rr := postIngest(t, h, IngestPayload{
		Source: SourceSDKHook,
		Spawn: &SpawnPayload{
			ID:        spawnID,
			Skill:     "ballard",
			StartedAt: ts,
			Project:   "tracelab",
		},
		Tokens: []TokensPayload{{
			SpawnID:      spawnID,
			InputTokens:  100,
			OutputTokens: 200,
			TS:           ts,
		}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("sdk-hook status=%d body=%s", rr.Code, rr.Body.String())
	}

	// (2) Transcript-tail picks up the same spawn from the JSONL and
	// reports slightly different token counts (10 % drift is a
	// realistic mismatch between the hook scope and the transcript
	// scope — exact disagreement is the forensics-interesting case).
	rr = postIngest(t, h, IngestPayload{
		Source: SourceTranscript,
		Tokens: []TokensPayload{{
			SpawnID:      spawnID,
			InputTokens:  110,
			OutputTokens: 220,
			TS:           ts,
		}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("transcript status=%d body=%s", rr.Code, rr.Body.String())
	}

	// (3) MCP-push (Phase-2d S3) — the worker itself reports yet
	// another set of counts via the new agent_event tool path.
	rr = postIngest(t, h, IngestPayload{
		Source: SourceMCPPush,
		Tokens: []TokensPayload{{
			SpawnID:      spawnID,
			InputTokens:  105,
			OutputTokens: 210,
			TS:           ts,
		}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("mcp-push status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Schema assertion 1: a single spawn row survives across the
	// three calls. PK collision (INSERT OR IGNORE on agent_spawns.id)
	// collapses the second + third writers' spawn payloads (or
	// missing-spawn payloads in this test) to a no-op.
	var spawnCount int
	if err := st.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM agent_spawns WHERE id = ?`, spawnID,
	).Scan(&spawnCount); err != nil {
		t.Fatalf("query spawns: %v", err)
	}
	if spawnCount != 1 {
		t.Errorf("agent_spawns rows for spawn %q: want 1, got %d (3 writers must dedupe on PK)",
			spawnID, spawnCount)
	}

	// Schema assertion 2: three distinct rows in agent_tokens because
	// source is part of the UNIQUE tuple (spawn_id, ts, source).
	// Per-source forensic breakdown intact — operators can read out
	// "sdk-hook said 100/200, transcript said 110/220, mcp-push said
	// 105/210" without losing any single writer's number.
	type tokenRow struct {
		Source       string
		InputTokens  int64
		OutputTokens int64
	}
	rows, err := st.DB().QueryContext(context.Background(),
		`SELECT source, input_tokens, output_tokens
		   FROM agent_tokens
		  WHERE spawn_id = ? AND ts = ?
		  ORDER BY source`,
		spawnID, ts)
	if err != nil {
		t.Fatalf("query tokens: %v", err)
	}
	defer rows.Close()
	var got []tokenRow
	for rows.Next() {
		var r tokenRow
		if err := rows.Scan(&r.Source, &r.InputTokens, &r.OutputTokens); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("agent_tokens rows: want 3 (sdk-hook + transcript + mcp-push), got %d (rows=%+v)",
			len(got), got)
	}
	// ORDER BY source — alphabetical: mcp-push, sdk-hook, transcript.
	want := []tokenRow{
		{Source: "mcp-push", InputTokens: 105, OutputTokens: 210},
		{Source: "sdk-hook", InputTokens: 100, OutputTokens: 200},
		{Source: "transcript", InputTokens: 110, OutputTokens: 220},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("agent_tokens[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Schema assertion 3: the source CHECK constraint accepts all
	// three values without rejection — implicit by the three 200
	// responses above. Lockstep with the migration's
	//   CHECK (source IN ('sdk-hook','transcript','mcp-push'))
	// in 0003_agents_schema.up.sql.
}
