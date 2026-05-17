package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// newTestHandler wires up an in-memory (well: temp-file) store, opens
// it (running all migrations), and returns a Handler ready to receive
// requests. Mirrors internal/store/sqlite_test.newTestStore.
func newTestHandler(t *testing.T) (*Handler, *store.Store) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "agents.db")
	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := NewHandler(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return h, st
}

// postIngest fires a POST /agents/ingest against the handler under
// test using httptest.NewRecorder. Returns the recorder so callers
// can inspect Code + Body.
func postIngest(t *testing.T, h *Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/agents/ingest", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Ingest(rr, req)
	return rr
}

// decodeIngestResponse parses the JSON body. Test helper.
func decodeIngestResponse(t *testing.T, rr *httptest.ResponseRecorder) ingestResponse {
	t.Helper()
	var resp ingestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response body %q: %v", rr.Body.String(), err)
	}
	return resp
}

// sampleSpawn returns a deterministic spawn-payload usable across
// tests. ID is fixed so two calls with this payload exercise the
// PK-collision idempotency path.
func sampleSpawn() *SpawnPayload {
	return &SpawnPayload{
		ID:        "01234567890123456789abcdef",
		Skill:     "ballard",
		StartedAt: time.Unix(1700000000, 0).UnixNano(),
		Project:   "tracelab",
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────

func TestAgentsIngestSmoke(t *testing.T) {
	h, st := newTestHandler(t)

	rr := postIngest(t, h, IngestPayload{
		Source: SourceSDKHook,
		Spawn:  sampleSpawn(),
		Tokens: []TokensPayload{{
			SpawnID:      sampleSpawn().ID,
			InputTokens:  100,
			OutputTokens: 200,
			TS:           time.Unix(1700000001, 0).UnixNano(),
		}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	resp := decodeIngestResponse(t, rr)
	if resp.Ingested.Spawns != 1 {
		t.Errorf("Ingested.Spawns: want 1, got %d", resp.Ingested.Spawns)
	}
	if resp.Ingested.Tokens != 1 {
		t.Errorf("Ingested.Tokens: want 1, got %d", resp.Ingested.Tokens)
	}

	// Verify the spawn row landed in the DB.
	var count int
	if err := st.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM agent_spawns WHERE id = ?`,
		sampleSpawn().ID).Scan(&count); err != nil {
		t.Fatalf("query spawns: %v", err)
	}
	if count != 1 {
		t.Errorf("agent_spawns row count: want 1, got %d", count)
	}
}

func TestAgentsIngestIdempotency(t *testing.T) {
	h, st := newTestHandler(t)

	payload := IngestPayload{
		Source: SourceSDKHook,
		Spawn:  sampleSpawn(),
		Tokens: []TokensPayload{{
			SpawnID: sampleSpawn().ID,
			TS:      time.Unix(1700000001, 0).UnixNano(),
		}},
	}

	rr1 := postIngest(t, h, payload)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first call status: want 200, got %d", rr1.Code)
	}
	resp1 := decodeIngestResponse(t, rr1)
	if resp1.Ingested.Spawns != 1 || resp1.Ingested.Tokens != 1 {
		t.Fatalf("first call counts: want 1/1, got %d/%d",
			resp1.Ingested.Spawns, resp1.Ingested.Tokens)
	}

	// Repeat. INSERT OR IGNORE collapses to no-op on both PK
	// (spawn) and UNIQUE-tuple (tokens(spawn_id,ts,source)).
	rr2 := postIngest(t, h, payload)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second call status: want 200, got %d", rr2.Code)
	}
	resp2 := decodeIngestResponse(t, rr2)
	if resp2.Ingested.Spawns != 0 {
		t.Errorf("second call Spawns: want 0 (collapsed), got %d", resp2.Ingested.Spawns)
	}
	if resp2.Ingested.Tokens != 0 {
		t.Errorf("second call Tokens: want 0 (collapsed), got %d", resp2.Ingested.Tokens)
	}

	// DB should still have exactly one row for each table.
	for _, table := range []string{"agent_spawns", "agent_tokens"} {
		var n int
		if err := st.DB().QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("%s row count after idempotent retry: want 1, got %d", table, n)
		}
	}
}

func TestAgentsIngestMultiSourceCoexistence(t *testing.T) {
	h, st := newTestHandler(t)

	// Same spawn pushed by SDK-hook.
	rr1 := postIngest(t, h, IngestPayload{
		Source: SourceSDKHook,
		Spawn:  sampleSpawn(),
		Tokens: []TokensPayload{{
			SpawnID:      sampleSpawn().ID,
			InputTokens:  100,
			OutputTokens: 200,
			TS:           time.Unix(1700000001, 0).UnixNano(),
		}},
	})
	if rr1.Code != http.StatusOK {
		t.Fatalf("sdk-hook call status: want 200, got %d (body=%s)", rr1.Code, rr1.Body.String())
	}

	// Same spawn + same ts, but transcript-tail reports DIFFERENT
	// token counts. The UNIQUE-tuple is (spawn_id, ts, source) —
	// distinct source means both rows survive. This is the
	// per-source forensics contract from ADR-013.
	rr2 := postIngest(t, h, IngestPayload{
		Source: SourceTranscript,
		Tokens: []TokensPayload{{
			SpawnID:      sampleSpawn().ID,
			InputTokens:  120, // disagreement!
			OutputTokens: 220,
			TS:           time.Unix(1700000001, 0).UnixNano(),
		}},
	})
	if rr2.Code != http.StatusOK {
		t.Fatalf("transcript call status: want 200, got %d (body=%s)", rr2.Code, rr2.Body.String())
	}
	resp2 := decodeIngestResponse(t, rr2)
	if resp2.Ingested.Tokens != 1 {
		t.Errorf("transcript call Tokens: want 1 (new row, different source), got %d",
			resp2.Ingested.Tokens)
	}

	// Expect TWO rows in agent_tokens for the same spawn+ts pair —
	// one per source.
	var sources []string
	rows, err := st.DB().QueryContext(context.Background(),
		`SELECT source FROM agent_tokens WHERE spawn_id = ? AND ts = ? ORDER BY source`,
		sampleSpawn().ID, time.Unix(1700000001, 0).UnixNano())
	if err != nil {
		t.Fatalf("query tokens: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		sources = append(sources, s)
	}
	if len(sources) != 2 {
		t.Fatalf("agent_tokens row count: want 2 (per-source forensics), got %d (sources=%v)",
			len(sources), sources)
	}
	if sources[0] != "sdk-hook" || sources[1] != "transcript" {
		t.Errorf("source ordering: want [sdk-hook transcript], got %v", sources)
	}
}

func TestAgentsIngestBadSource(t *testing.T) {
	h, _ := newTestHandler(t)

	// Raw body — can't use the typed Source type for this one
	// (compile-time enum-ish guard would refuse).
	body := []byte(`{"source": "unknown", "tokens": [{"spawn_id": "x", "ts": 1700000000000000000}]}`)
	req := httptest.NewRequest(http.MethodPost, "/agents/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "unknown source") {
		t.Errorf("body should mention 'unknown source': %s", rr.Body.String())
	}
}

func TestAgentsIngestEmptyPayload(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := postIngest(t, h, IngestPayload{Source: SourceSDKHook})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty-payload status: want 400, got %d (body=%s)",
			rr.Code, rr.Body.String())
	}
}

func TestAgentsIngestUnknownVerdict(t *testing.T) {
	h, _ := newTestHandler(t)

	rr := postIngest(t, h, IngestPayload{
		Source: SourceMCPPush,
		Verdicts: []VerdictPayload{{
			SpawnID: sampleSpawn().ID,
			Verdict: "approved", // not in the enum
			TS:      time.Unix(1700000000, 0).UnixNano(),
		}},
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d (body=%s)", rr.Code, rr.Body.String())
	}
}

func TestAgentsIngestVerdictAndEdges(t *testing.T) {
	h, st := newTestHandler(t)

	// Need two spawns to wire an edge between them.
	spawnA := sampleSpawn()
	spawnB := *sampleSpawn()
	spawnB.ID = "01234567890123456789ffffff"

	rr1 := postIngest(t, h, IngestPayload{Source: SourceMCPPush, Spawn: spawnA})
	if rr1.Code != http.StatusOK {
		t.Fatalf("spawn A status: %d (%s)", rr1.Code, rr1.Body.String())
	}
	rr2 := postIngest(t, h, IngestPayload{Source: SourceMCPPush, Spawn: &spawnB})
	if rr2.Code != http.StatusOK {
		t.Fatalf("spawn B status: %d (%s)", rr2.Code, rr2.Body.String())
	}

	// Push a verdict + edge in one call.
	rr3 := postIngest(t, h, IngestPayload{
		Source: SourceMCPPush,
		Verdicts: []VerdictPayload{{
			SpawnID:      spawnA.ID,
			Verdict:      "freigabe",
			LerneffektMD: "all good",
			TS:           time.Unix(1700000010, 0).UnixNano(),
		}},
		MailboxEdges: []MailboxEdgePayload{{
			FromSpawnID: spawnA.ID,
			ToSpawnID:   spawnB.ID,
			EdgeType:    "delegate",
			TS:          time.Unix(1700000011, 0).UnixNano(),
		}},
	})
	if rr3.Code != http.StatusOK {
		t.Fatalf("verdict+edge status: %d (%s)", rr3.Code, rr3.Body.String())
	}
	resp := decodeIngestResponse(t, rr3)
	if resp.Ingested.Verdicts != 1 || resp.Ingested.MailboxEdges != 1 {
		t.Errorf("counts: want verdicts=1 edges=1, got %+v", resp.Ingested)
	}

	// Verify DB.
	var v, e int
	_ = st.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM agent_verdicts WHERE spawn_id = ?`, spawnA.ID).Scan(&v)
	_ = st.DB().QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM agent_mailbox_edges WHERE from_spawn_id = ?`,
		spawnA.ID).Scan(&e)
	if v != 1 || e != 1 {
		t.Errorf("DB counts: want verdicts=1 edges=1, got verdicts=%d edges=%d", v, e)
	}
}

func TestAgentsIngestInvalidJSON(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/agents/ingest",
		bytes.NewReader([]byte(`{"source": "sdk-hook"`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", rr.Code)
	}
}

func TestAgentsIngestUnknownField(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/agents/ingest",
		bytes.NewReader([]byte(`{"source": "sdk-hook", "unknown_field": 42}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("unknown field status: want 400 (DisallowUnknownFields), got %d", rr.Code)
	}
}
