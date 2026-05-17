package agents

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// openTestStore opens a fresh on-disk store in t.TempDir() so we get
// the same migration chain (incl. 0003 agent_*) the real hub uses.
// We deliberately use file:// rather than :memory: because the embed
// migration runner has been validated against the on-disk path in S1.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// loadSampleFixture reads scripts/transcript-sample.jsonl from the
// repo. Tests live at internal/agents/transcript_test.go so the
// fixture path is "../../scripts/transcript-sample.jsonl".
func loadSampleFixture(t *testing.T) []byte {
	t.Helper()
	const rel = "../../scripts/transcript-sample.jsonl"
	data, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read sample fixture: %v", err)
	}
	return data
}

func TestTranscriptParseJSONL(t *testing.T) {
	t.Parallel()
	data := loadSampleFixture(t)
	lines := splitJSONL(data)
	if len(lines) < 4 {
		t.Fatalf("sample fixture must contain at least 4 lines, got %d", len(lines))
	}

	var (
		gotSpawns   int
		gotTokens   int
		gotVerdicts int
	)
	for i, line := range lines {
		events, err := parseTranscriptLine(line)
		if err != nil {
			t.Fatalf("line %d parse: %v", i, err)
		}
		for _, e := range events {
			switch {
			case e.Spawn != nil:
				gotSpawns++
				if len(e.Spawn.ID) != 26 {
					t.Errorf("line %d spawn id length = %d, want 26", i, len(e.Spawn.ID))
				}
				if e.Spawn.Project == "" {
					t.Errorf("line %d spawn project empty", i)
				}
				if e.Spawn.StartedAt.IsZero() {
					t.Errorf("line %d spawn started_at zero", i)
				}
			case e.Tokens != nil:
				gotTokens++
				if e.Tokens.Source != string(SourceTranscript) {
					t.Errorf("line %d tokens source = %q, want transcript", i, e.Tokens.Source)
				}
				if len(e.Tokens.SpawnID) != 26 {
					t.Errorf("line %d tokens spawn_id len = %d, want 26", i, len(e.Tokens.SpawnID))
				}
			case e.Verdict != nil:
				gotVerdicts++
				if !validVerdicts[e.Verdict.Verdict] {
					t.Errorf("line %d verdict %q not in CHECK vocabulary", i, e.Verdict.Verdict)
				}
			}
		}
	}
	if gotSpawns == 0 {
		t.Errorf("expected at least one spawn event from sample fixture")
	}
	if gotTokens == 0 {
		t.Errorf("expected at least one token event from sample fixture")
	}
	if gotVerdicts == 0 {
		t.Errorf("expected at least one verdict event from sample fixture")
	}
}

func TestTranscriptParseCorruptedLineReturnsError(t *testing.T) {
	t.Parallel()
	_, err := parseTranscriptLine([]byte(`{"type":"assistant", garbage`))
	if err == nil {
		t.Fatalf("expected JSON decode error on corrupted line")
	}
}

func TestTranscriptParseEmptyOrNoTypeYieldsNoEvents(t *testing.T) {
	t.Parallel()
	events, err := parseTranscriptLine([]byte(`{}`))
	if err != nil {
		t.Fatalf("empty record parse: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("empty-type record produced %d events, want 0", len(events))
	}
	events, err = parseTranscriptLine([]byte{})
	if err != nil {
		t.Fatalf("zero-length parse: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("zero-length input produced %d events, want 0", len(events))
	}
}

func TestPadTo26(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"", "aaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"abc", "abcaaaaaaaaaaaaaaaaaaaaaaa"},
		{strings.Repeat("a", 26), strings.Repeat("a", 26)},
		{strings.Repeat("b", 40), strings.Repeat("b", 26)},
	}
	for _, tc := range cases {
		got := padTo26(tc.in)
		if got != tc.want {
			t.Errorf("padTo26(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if len(got) != 26 {
			t.Errorf("padTo26 result len = %d, want 26", len(got))
		}
	}
}

func TestTranscriptTailLoopLineBufferingRobust(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "-home-kaik-Projekte-tracelab")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonlPath := filepath.Join(projectDir, "sess1.jsonl")

	// Phase 1: write 2 complete lines + 1 partial (no trailing \n).
	// All agent IDs are exactly 26 chars to bypass padTo26's truncation.
	id1 := "ts1aaaaaaaaaaaaaaaaaaaaaa1" // 26
	id2 := "ts2aaaaaaaaaaaaaaaaaaaaaa2" // 26
	completeRec1 := assistantRecordJSON(t, id1, "2026-05-17T10:00:00.000Z", 10, 20)
	completeRec2 := assistantRecordJSON(t, id2, "2026-05-17T10:00:01.000Z", 30, 40)
	partial := `{"type":"assistant","sessionId":"part","timestamp":"2026-05-17T10:00:02.000Z","mes` // intentionally truncated
	if err := os.WriteFile(jsonlPath,
		[]byte(completeRec1+"\n"+completeRec2+"\n"+partial),
		0o644,
	); err != nil {
		t.Fatalf("write: %v", err)
	}

	br, err := NewTranscriptBridge(TranscriptBridgeDeps{
		Store:        st,
		Logger:       silentLogger(),
		ProjectsRoot: dir,
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTranscriptBridge: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// One manual tick: simpler than racing the loop, exactly what
	// the tick-loop would call.
	br.tickOnce(ctx)

	// We expect 2 token rows (one per complete record). The partial
	// line was NOT consumed (offset bookkeeping rule).
	tokensRows := countTokenRows(t, st, id1) + countTokenRows(t, st, id2)
	if tokensRows != 2 {
		t.Errorf("after phase 1: token rows = %d, want 2", tokensRows)
	}

	// Phase 2: complete the partial line + add a third line.
	id3 := "ts3aaaaaaaaaaaaaaaaaaaaaa3"
	completeRec3 := assistantRecordJSON(t, id3, "2026-05-17T10:00:03.000Z", 50, 60)
	rest := `sage":{"usage":{"input_tokens":1,"output_tokens":2}}}` + "\n" + completeRec3 + "\n"
	f, err := os.OpenFile(jsonlPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, err := io.WriteString(f, rest); err != nil {
		t.Fatalf("append: %v", err)
	}
	_ = f.Close()

	br.tickOnce(ctx)

	// Phase 2 should add at minimum the third record. The completed
	// "part" line has session-derived spawn id which will also yield
	// a token row — both are fine; we assert the third record landed.
	if got := countTokenRows(t, st, id3); got != 1 {
		t.Errorf("after phase 2: ts3 token rows = %d, want 1", got)
	}
}

func TestTranscriptMultiSourceCoexistence(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	ctx := context.Background()

	const spawnID = "multisourcespawnid00000abc"
	ts := time.Unix(0, 1_700_000_000_000_000_000)

	// 1) Plant a spawn row so both subsequent token inserts have an FK.
	if _, err := st.InsertAgentSpawn(ctx, store.AgentSpawn{
		ID:        spawnID,
		Skill:     "ballard",
		Project:   "tracelab",
		StartedAt: ts.Add(-time.Second),
	}); err != nil {
		t.Fatalf("seed spawn: %v", err)
	}

	// 2) SDK-hook source plants tokens at (spawn, ts).
	if _, err := st.InsertAgentTokens(ctx, store.AgentTokenUsage{
		SpawnID:      spawnID,
		InputTokens:  100,
		OutputTokens: 200,
		TS:           ts,
		Source:       string(SourceSDKHook),
	}); err != nil {
		t.Fatalf("sdk-hook tokens: %v", err)
	}

	// 3) Transcript source plants tokens at the SAME (spawn, ts) but
	//    different source. Per ADR-013 §Consequences the UNIQUE-tuple
	//    on (spawn_id, ts, source) lets both rows coexist.
	if _, err := st.InsertAgentTokens(ctx, store.AgentTokenUsage{
		SpawnID:      spawnID,
		InputTokens:  110,
		OutputTokens: 220,
		TS:           ts,
		Source:       string(SourceTranscript),
	}); err != nil {
		t.Fatalf("transcript tokens: %v", err)
	}

	// Assertions:
	//  - exactly ONE row in agent_spawns (PK on id).
	//  - exactly TWO rows in agent_tokens for this spawn (one per source).
	spawnCount := countSpawnRows(t, st, spawnID)
	if spawnCount != 1 {
		t.Errorf("spawn rows = %d, want 1 (PK collapse)", spawnCount)
	}
	tokenCount := countTokenRows(t, st, spawnID)
	if tokenCount != 2 {
		t.Errorf("token rows = %d, want 2 (per-source forensics)", tokenCount)
	}

	// 4) Idempotency: replaying the same SDK-hook event is a no-op.
	n, err := st.InsertAgentTokens(ctx, store.AgentTokenUsage{
		SpawnID: spawnID, InputTokens: 100, OutputTokens: 200,
		TS: ts, Source: string(SourceSDKHook),
	})
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if n != 0 {
		t.Errorf("idempotent replay rows = %d, want 0", n)
	}
}

func TestTranscriptTailCorruptedLineSkipped(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "corrupt.jsonl")
	goodID := "goodaaaaaaaaaaaaaaaaaaaaa1" // 26
	good2ID := "good2aaaaaaaaaaaaaaaaaaaa2" // 26
	good := assistantRecordJSON(t, goodID, "2026-05-17T11:00:00.000Z", 7, 8)
	corrupt := `{"type":"assistant", broken json `
	good2 := assistantRecordJSON(t, good2ID, "2026-05-17T11:00:01.000Z", 9, 10)
	content := good + "\n" + corrupt + "\n" + good2 + "\n"
	if err := os.WriteFile(jsonlPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	br, err := NewTranscriptBridge(TranscriptBridgeDeps{
		Store: st, Logger: silentLogger(),
		ProjectsRoot: dir, PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTranscriptBridge: %v", err)
	}
	br.tickOnce(context.Background())

	if got := countTokenRows(t, st, goodID); got != 1 {
		t.Errorf("good before corrupt: token rows = %d, want 1", got)
	}
	if got := countTokenRows(t, st, good2ID); got != 1 {
		t.Errorf("good after corrupt: token rows = %d, want 1 (corrupt line must not desync)", got)
	}
}

func TestTranscriptTailRunCancelsCleanly(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	br, err := NewTranscriptBridge(TranscriptBridgeDeps{
		Store: st, Logger: silentLogger(),
		ProjectsRoot: t.TempDir(),
		PollInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTranscriptBridge: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- br.Run(ctx) }()
	// Let one tick happen, then cancel.
	time.Sleep(120 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil on graceful cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within 2s of cancel")
	}
}

func TestNewTranscriptBridgeRejectsNilStore(t *testing.T) {
	t.Parallel()
	if _, err := NewTranscriptBridge(TranscriptBridgeDeps{
		ProjectsRoot: "/tmp",
	}); err == nil {
		t.Fatalf("expected error for nil store")
	}
}

func TestExpandHomeTilde(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}
	got, err := expandHome("~/foo/bar")
	if err != nil {
		t.Fatalf("expandHome: %v", err)
	}
	want := filepath.Join(home, "foo/bar")
	if got != want {
		t.Errorf("expandHome(~/foo/bar) = %q, want %q", got, want)
	}
	got, err = expandHome("/abs")
	if err != nil {
		t.Fatalf("expandHome abs: %v", err)
	}
	if got != "/abs" {
		t.Errorf("expandHome(/abs) = %q, want /abs (no rewrite)", got)
	}
}

// ===== helpers =====

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// splitJSONL splits a JSONL blob on newlines, dropping empty lines.
func splitJSONL(data []byte) [][]byte {
	var out [][]byte
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, []byte(l))
	}
	return out
}

// assistantRecordJSON encodes a minimal type=assistant record with a
// usage block. Used by the tail-loop tests to plant deterministic
// input lines.
func assistantRecordJSON(t *testing.T, agentID, ts string, in, out int64) string {
	t.Helper()
	rec := map[string]any{
		"type":        "assistant",
		"isSidechain": true,
		"agentId":     agentID,
		"sessionId":   "tail-test-session",
		"cwd":         "/home/kaik/Projekte/tracelab",
		"timestamp":   ts,
		"uuid":        "uuid-" + agentID,
		"message": map[string]any{
			"role": "assistant",
			"id":   "msg-" + agentID,
			"usage": map[string]any{
				"input_tokens":  in,
				"output_tokens": out,
			},
		},
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal helper: %v", err)
	}
	return string(b)
}

// countSpawnRows counts rows in agent_spawns matching spawnID.
func countSpawnRows(t *testing.T, st *store.Store, spawnID string) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM agent_spawns WHERE id = ?`, spawnID).Scan(&n); err != nil {
		t.Fatalf("count spawns: %v", err)
	}
	return n
}

// countTokenRows counts rows in agent_tokens matching spawnID.
func countTokenRows(t *testing.T, st *store.Store, spawnID string) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM agent_tokens WHERE spawn_id = ?`, spawnID).Scan(&n); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	return n
}
