// read_test.go — Phase 2d S4 acceptance tests for the four
// /agents/{sessions,tree,tokens,verdicts} read endpoints.
//
// Coverage:
//
//   - TestAgentsReadMuxBearerAuth        → 401 without bearer, 200 with
//   - TestAgentsReadMuxPassThrough       → POST /agents/ingest and
//                                          non-/agents paths reach `next`
//   - TestAgentsReadSessionsEmpty        → empty list shape
//   - TestAgentsReadSessionsAndPaging    → seeded list + paging
//   - TestAgentsReadTreeBFSOrder         → root + children + depth
//   - TestAgentsReadTreeUnknown404       → unknown root → 404
//   - TestAgentsReadTokensThreeSources   → three-source breakdown shape
//   - TestAgentsReadTokensMissingParam   → 400 on missing spawn_id
//   - TestAgentsReadVerdictsMultiple     → ts-ASC ordering, latest last

package agents

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

const testBearerToken = "test-secret-token"

// newReadEnv wires up a Handler + in-process httptest server with the
// AgentsReadMux mounted in front of a sentinel next-handler that
// records whether a request was passed through.
type readEnv struct {
	h            *Handler
	st           *store.Store
	srv          *httptest.Server
	passthroughs int
}

func newReadEnv(t *testing.T) *readEnv {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "agents-read.db")
	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	env := &readEnv{st: st}
	env.h = NewHandler(st, slog.New(slog.NewTextHandler(io.Discard, nil)))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env.passthroughs++
		w.Header().Set("X-Passthrough", "1")
		w.WriteHeader(http.StatusTeapot) // sentinel marker
	})
	mux := env.h.AgentsReadMux(testBearerToken, next)
	env.srv = httptest.NewServer(mux)
	t.Cleanup(env.srv.Close)
	return env
}

func bearerGET(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testBearerToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

// seedSpawnRow inserts a spawn through the public store API.
func seedSpawnRow(t *testing.T, st *store.Store, id, parent, skill, project string, started time.Time) {
	t.Helper()
	sp := store.AgentSpawn{
		ID:        id,
		Skill:     skill,
		Project:   project,
		StartedAt: started,
	}
	if parent != "" {
		sp.ParentID = parent
	}
	if _, err := st.InsertAgentSpawn(context.Background(), sp); err != nil {
		t.Fatalf("InsertAgentSpawn(%s): %v", id, err)
	}
}

func seedTokenRow(t *testing.T, st *store.Store, spawn string, in, out, cr, cw int64, ts time.Time, source string) {
	t.Helper()
	if _, err := st.InsertAgentTokens(context.Background(), store.AgentTokenUsage{
		SpawnID:      spawn,
		InputTokens:  in,
		OutputTokens: out,
		CacheRead:    cr,
		CacheWrite:   cw,
		TS:           ts,
		Source:       source,
	}); err != nil {
		t.Fatalf("InsertAgentTokens: %v", err)
	}
}

func seedVerdictRow(t *testing.T, st *store.Store, spawn, verdict, lerneffekt string, ts time.Time) {
	t.Helper()
	if _, err := st.InsertAgentVerdict(context.Background(), store.AgentVerdict{
		SpawnID:      spawn,
		Verdict:      verdict,
		LerneffektMD: lerneffekt,
		TS:           ts,
	}); err != nil {
		t.Fatalf("InsertAgentVerdict: %v", err)
	}
}

func TestAgentsReadMuxBearerAuth(t *testing.T) {
	env := newReadEnv(t)

	// No header → 401.
	resp, err := http.Get(env.srv.URL + "/agents/sessions")
	if err != nil {
		t.Fatalf("GET no-auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth: status=%d, want 401", resp.StatusCode)
	}

	// Wrong header → 401.
	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/agents/sessions", nil)
	req.Header.Set("Authorization", "Bearer not-the-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET wrong-auth: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong-auth: status=%d, want 401", resp2.StatusCode)
	}

	// Correct header → 200 (empty list).
	resp3 := bearerGET(t, env.srv, "/agents/sessions")
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("good-auth: status=%d, want 200", resp3.StatusCode)
	}
}

func TestAgentsReadMuxPassThrough(t *testing.T) {
	env := newReadEnv(t)

	// Non-/agents path passes through.
	resp, err := http.Get(env.srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.Header.Get("X-Passthrough") != "1" {
		t.Errorf("non-/agents path did NOT pass through to next-handler")
	}

	// POST /agents/ingest passes through (the read mux only handles GETs).
	req, _ := http.NewRequest(http.MethodPost, env.srv.URL+"/agents/ingest", nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST ingest: %v", err)
	}
	resp2.Body.Close()
	if resp2.Header.Get("X-Passthrough") != "1" {
		t.Errorf("POST /agents/ingest did NOT pass through")
	}
}

func TestAgentsReadSessionsEmpty(t *testing.T) {
	env := newReadEnv(t)
	resp := bearerGET(t, env.srv, "/agents/sessions")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body spawnsListResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Total != 0 {
		t.Errorf("Total=%d, want 0", body.Total)
	}
	if len(body.Spawns) != 0 {
		t.Errorf("Spawns=%v, want empty", body.Spawns)
	}
}

func TestAgentsReadSessionsAndPaging(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	seedSpawnRow(t, env.st, "a1aaaaaaaaaaaaaaaaaaaaaaaa", "", "ballard", "tracelab", now)
	seedSpawnRow(t, env.st, "a2aaaaaaaaaaaaaaaaaaaaaaaa", "", "tuvok", "tracelab", now.Add(time.Second))
	seedSpawnRow(t, env.st, "a3aaaaaaaaaaaaaaaaaaaaaaaa", "", "barclay", "nexus", now.Add(2*time.Second))

	resp := bearerGET(t, env.srv, "/agents/sessions?limit=2")
	defer resp.Body.Close()
	var body spawnsListResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Total != 3 {
		t.Errorf("Total=%d, want 3", body.Total)
	}
	if len(body.Spawns) != 2 {
		t.Errorf("Spawns len=%d, want 2", len(body.Spawns))
	}
	// Newest first: a3 then a2
	if body.Spawns[0].ID != "a3aaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("first row=%q, want a3...", body.Spawns[0].ID)
	}

	// Filter by project → only tracelab spawns.
	resp2 := bearerGET(t, env.srv, "/agents/sessions?project=tracelab")
	defer resp2.Body.Close()
	var body2 spawnsListResp
	if err := json.NewDecoder(resp2.Body).Decode(&body2); err != nil {
		t.Fatalf("decode2: %v", err)
	}
	if body2.Total != 2 {
		t.Errorf("filtered Total=%d, want 2", body2.Total)
	}
	for _, sp := range body2.Spawns {
		if sp.Project != "tracelab" {
			t.Errorf("filter leak: project=%q", sp.Project)
		}
	}
}

func TestAgentsReadTreeBFSOrder(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	// root → child1, child2; child1 → grandchild
	rootID := "rootspawn11aaaaaaaaaaaaaaa"
	c1 := "child1spawn1aaaaaaaaaaaaaa"
	c2 := "child2spawn1aaaaaaaaaaaaaa"
	gc := "grandchildspaaaaaaaaaaaaaa"
	seedSpawnRow(t, env.st, rootID, "", "belanna", "tracelab", now)
	seedSpawnRow(t, env.st, c1, rootID, "ballard", "tracelab", now.Add(time.Second))
	seedSpawnRow(t, env.st, c2, rootID, "tuvok", "tracelab", now.Add(2*time.Second))
	seedSpawnRow(t, env.st, gc, c1, "harren", "tracelab", now.Add(3*time.Second))

	resp := bearerGET(t, env.srv, "/agents/tree/"+rootID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body treeResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Root != rootID {
		t.Errorf("Root=%q, want %q", body.Root, rootID)
	}
	if len(body.Nodes) != 4 {
		t.Errorf("Nodes len=%d, want 4", len(body.Nodes))
	}
	// BFS order: root, c1, c2, gc
	wantIDs := []string{rootID, c1, c2, gc}
	wantDepths := []int{0, 1, 1, 2}
	for i, n := range body.Nodes {
		if n.ID != wantIDs[i] {
			t.Errorf("Nodes[%d].ID=%q, want %q", i, n.ID, wantIDs[i])
		}
		if n.Depth != wantDepths[i] {
			t.Errorf("Nodes[%d].Depth=%d, want %d", i, n.Depth, wantDepths[i])
		}
	}
}

func TestAgentsReadTreeUnknown404(t *testing.T) {
	env := newReadEnv(t)
	resp := bearerGET(t, env.srv, "/agents/tree/doesnotexist1aaaaaaaaaaaa")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
}

func TestAgentsReadTokensThreeSources(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	spawnID := "tokenspawn1aaaaaaaaaaaaaaa"
	seedSpawnRow(t, env.st, spawnID, "", "ballard", "tracelab", now)
	seedTokenRow(t, env.st, spawnID, 100, 50, 0, 0, now, "sdk-hook")
	seedTokenRow(t, env.st, spawnID, 200, 100, 0, 0, now.Add(time.Millisecond), "transcript")
	seedTokenRow(t, env.st, spawnID, 300, 150, 0, 0, now.Add(2*time.Millisecond), "mcp-push")

	resp := bearerGET(t, env.srv, "/agents/tokens?spawn_id="+spawnID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body tokensResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SpawnID != spawnID {
		t.Errorf("SpawnID=%q, want %q", body.SpawnID, spawnID)
	}
	if len(body.Tokens) != 3 {
		t.Errorf("Tokens len=%d, want 3 (sdk-hook + transcript + mcp-push)", len(body.Tokens))
	}
	// Cross-source totals
	if body.Totals.InputTokens != 600 {
		t.Errorf("Totals.InputTokens=%d, want 600", body.Totals.InputTokens)
	}
	if body.Totals.OutputTokens != 300 {
		t.Errorf("Totals.OutputTokens=%d, want 300", body.Totals.OutputTokens)
	}
	// Per-source breakdown
	if len(body.Totals.BySource) != 3 {
		t.Errorf("BySource keys=%d, want 3", len(body.Totals.BySource))
	}
	if body.Totals.BySource["sdk-hook"].InputTokens != 100 {
		t.Errorf("sdk-hook breakdown wrong: %+v", body.Totals.BySource["sdk-hook"])
	}
	if body.Totals.BySource["transcript"].InputTokens != 200 {
		t.Errorf("transcript breakdown wrong: %+v", body.Totals.BySource["transcript"])
	}
	if body.Totals.BySource["mcp-push"].InputTokens != 300 {
		t.Errorf("mcp-push breakdown wrong: %+v", body.Totals.BySource["mcp-push"])
	}
}

func TestAgentsReadTokensMissingParam(t *testing.T) {
	env := newReadEnv(t)
	resp := bearerGET(t, env.srv, "/agents/tokens")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
}

func TestAgentsReadVerdictsMultiple(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	spawnID := "vspawn1aaaaaaaaaaaaaaaaaaa"
	seedSpawnRow(t, env.st, spawnID, "", "tuvok", "tracelab", now)
	// Two verdicts: first auflagen, then freigabe after fix.
	seedVerdictRow(t, env.st, spawnID, "auflagen", "Findings open: 2", now)
	seedVerdictRow(t, env.st, spawnID, "freigabe", "All findings cleared.", now.Add(time.Hour))

	resp := bearerGET(t, env.srv, "/agents/verdicts?spawn_id="+spawnID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body verdictsResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Verdicts) != 2 {
		t.Fatalf("Verdicts len=%d, want 2", len(body.Verdicts))
	}
	// ts-ASC: auflagen first, freigabe second
	if body.Verdicts[0].Verdict != "auflagen" {
		t.Errorf("Verdicts[0]=%q, want auflagen", body.Verdicts[0].Verdict)
	}
	if body.Verdicts[1].Verdict != "freigabe" {
		t.Errorf("Verdicts[1]=%q, want freigabe", body.Verdicts[1].Verdict)
	}
	if body.Verdicts[1].LerneffektMD != "All findings cleared." {
		t.Errorf("Verdicts[1].LerneffektMD=%q", body.Verdicts[1].LerneffektMD)
	}
}

// TestAgentsReadInvalidSpawnIDFormat verifies the path-traversal/charset
// defence on the path-/query-parameter readers.
func TestAgentsReadInvalidSpawnIDFormat(t *testing.T) {
	env := newReadEnv(t)
	// Embedded ".." should be rejected at the format-check layer.
	resp := bearerGET(t, env.srv, "/agents/tokens?spawn_id=foo..bar")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("path-traversal not rejected; status=%d", resp.StatusCode)
	}
}
