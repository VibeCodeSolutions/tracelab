// transcript_edges_test.go — Phase 2d S5 transcript-tail edge extraction.
//
// Two synthetic JSONL records:
//   - parent assistant message with a Task tool_use inside content[]
//     → emits a spawn edge from parent → child
//   - parent record with toolUseResult.{status,agentId,...}
//     → emits a return edge from child → parent
//
// We assert the edge events appear in the parser output AND that they
// land in the store via persistEvent + InsertAgentMailboxEdge.

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// makeAssistantWithTaskToolUse renders a JSONL line matching the
// shape produced by Claude-Code's transcript tail when an assistant
// message contains a Task tool_use. The toolUseId becomes the child
// spawn id (after stripNonHex + padTo26).
func makeAssistantWithTaskToolUse(t *testing.T, sessionID, cwd, ts, toolUseID string) []byte {
	t.Helper()
	rec := map[string]any{
		"type":      "assistant",
		"uuid":      "msg-uuid-test",
		"sessionId": sessionID,
		"cwd":       cwd,
		"timestamp": ts,
		"message": map[string]any{
			"id":   "m1",
			"role": "assistant",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
			"content": []map[string]any{
				{
					"type": "tool_use",
					"id":   toolUseID,
					"name": "Task",
					"input": map[string]any{
						"description": "ballard",
						"prompt":      "go build",
					},
				},
			},
		},
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// makeToolUseResult renders a parent-stream record carrying the spawn-
// end metadata for a finished subagent (agentId + status + usage).
func makeToolUseResult(t *testing.T, sessionID, cwd, ts, agentID, status string) []byte {
	t.Helper()
	rec := map[string]any{
		"type":      "user",
		"uuid":      "result-uuid",
		"sessionId": sessionID,
		"cwd":       cwd,
		"timestamp": ts,
		"toolUseResult": map[string]any{
			"status":    status,
			"agentId":   agentID,
			"agentType": "ballard",
			"usage": map[string]any{
				"input_tokens":  100,
				"output_tokens": 50,
			},
		},
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestTranscriptParseEmitsSpawnEdgeOnTaskToolUse(t *testing.T) {
	t.Parallel()
	const toolUseID = "toolu_01abcdef0123456789abcdef"
	line := makeAssistantWithTaskToolUse(t,
		"01234567-89ab-cdef-0123-456789abcdef",
		"/home/user/Projekte/tracelab",
		"2026-05-18T12:00:00Z",
		toolUseID,
	)
	events, err := parseTranscriptLine(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var (
		gotSpawnEdges int
		edgeFrom      string
		edgeTo        string
	)
	for _, e := range events {
		if e.Edge == nil {
			continue
		}
		if e.Edge.EdgeType == "spawn" {
			gotSpawnEdges++
			edgeFrom = e.Edge.FromSpawnID
			edgeTo = e.Edge.ToSpawnID
		}
	}
	if gotSpawnEdges != 1 {
		t.Fatalf("spawn-edges=%d, want 1 (events=%+v)", gotSpawnEdges, events)
	}
	if edgeFrom == "" || edgeTo == "" || edgeFrom == edgeTo {
		t.Errorf("edge endpoints invalid: from=%q to=%q", edgeFrom, edgeTo)
	}
}

func TestTranscriptParseEmitsReturnEdgeOnToolUseResult(t *testing.T) {
	t.Parallel()
	line := makeToolUseResult(t,
		"01234567-89ab-cdef-0123-456789abcdef",
		"/home/user/Projekte/tracelab",
		"2026-05-18T12:01:00Z",
		"agent-aabbccddeeff",
		"completed",
	)
	events, err := parseTranscriptLine(line)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var gotReturn int
	for _, e := range events {
		if e.Edge != nil && e.Edge.EdgeType == "return" {
			gotReturn++
		}
	}
	if gotReturn != 1 {
		t.Fatalf("return-edges=%d, want 1 (events=%+v)", gotReturn, events)
	}
}

func TestTranscriptPersistEventInsertsEdge(t *testing.T) {
	t.Parallel()
	st := newTestStoreForEdges(t)
	deps := TranscriptBridgeDeps{
		Store:        st,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		ProjectsRoot: t.TempDir(),
		PollInterval: time.Second,
	}
	br, err := NewTranscriptBridge(deps)
	if err != nil {
		t.Fatalf("NewTranscriptBridge: %v", err)
	}
	ctx := context.Background()

	// Seed two spawns (FK precondition).
	const parent = "parentaaaaaaaaaaaaaaaaaaaa"
	const child = "childaaaaaaaaaaaaaaaaaaaaa"
	for _, id := range []string{parent, child} {
		if _, err := st.InsertAgentSpawn(ctx, store.AgentSpawn{
			ID: id, Skill: "x", Project: "p", StartedAt: time.Unix(1700000000, 0),
		}); err != nil {
			t.Fatalf("seed spawn %s: %v", id, err)
		}
	}

	// Persist a spawn-edge event via the bridge's dispatch path.
	err = br.persistEvent(ctx, transcriptEvent{Edge: &store.AgentMailboxEdge{
		FromSpawnID: parent,
		ToSpawnID:   child,
		EdgeType:    "spawn",
		TS:          time.Unix(1700000001, 0),
	}})
	if err != nil {
		t.Fatalf("persistEvent: %v", err)
	}

	in, _, err := st.AgentEdgesForSpawn(ctx, child)
	if err != nil {
		t.Fatalf("AgentEdgesForSpawn: %v", err)
	}
	if len(in) != 1 || in[0].FromSpawnID != parent {
		t.Errorf("in=%+v, want one edge from parent", in)
	}
}

// newTestStoreForEdges opens a fresh on-disk store for the edge tests.
// Lives in this file (rather than reusing read_test.go's helpers)
// because read_test.go does not export newReadEnv outside that file's
// test surface; the helper is a 5-liner so the duplication cost is
// trivial.
func newTestStoreForEdges(t *testing.T) *store.Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), fmt.Sprintf("agents-edges-%d.db", time.Now().UnixNano()))
	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
