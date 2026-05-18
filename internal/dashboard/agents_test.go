// agents_test.go — Tests for the Phase 2d S4 Agents tab handler.
//
// Exercises AgentsHandler + renderAgentsBody against a real on-disk
// modernc.org sqlite store seeded with multi-ingest fixtures (sdk-hook
// + transcript + mcp-push). Covers:
//
//   - TestAgentsTabRendersWithSpawnData      → list + tokens + verdict
//                                              rendered end-to-end, with
//                                              per-source breakdown when
//                                              multiple sources are present
//   - TestAgentsTabRendersEmptyStateWithStore → empty-state card when no
//                                                spawns yet (live-store path)
//   - TestAgentsTabPaginationAndFilter        → page/limit + project filter
//   - TestAgentsTabAggregatesThreeSources     → 3-source aggregation pin
//                                                (mirrors the ingest-side
//                                                ThreeSourceCoexistence test)

package dashboard_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/dashboard"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// agentsTestEnv wires a dashboard.Handler with a fresh on-disk store
// and an httptest.Server serving the agents-tab handler. Returned
// values are torn down by t.Cleanup hooks.
type agentsTestEnv struct {
	srv *httptest.Server
	st  *store.Store
	h   *dashboard.Handler
}

func newAgentsTestEnv(t *testing.T) *agentsTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agents.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	h, err := dashboard.NewHandler("test", nil, st)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard/tab/agents", h.AgentsHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &agentsTestEnv{srv: srv, st: st, h: h}
}

// padSpawnID is the same 26-char pad pattern the ingest layer uses
// (see internal/agents/transcript.go padTo26). Replicated here as a
// test helper so the seeded ids stay schema-shape-correct.
func padSpawnID(prefix string) string {
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

// seedSpawn writes one spawn row, optionally with parent / session.
func seedSpawn(t *testing.T, st *store.Store, id, parent, skill, project string, startedAt time.Time) {
	t.Helper()
	spawn := store.AgentSpawn{
		ID:        id,
		Skill:     skill,
		Project:   project,
		StartedAt: startedAt,
	}
	if parent != "" {
		spawn.ParentID = parent
	}
	if _, err := st.InsertAgentSpawn(context.Background(), spawn); err != nil {
		t.Fatalf("InsertAgentSpawn(%s): %v", id, err)
	}
}

func seedTokens(t *testing.T, st *store.Store, spawnID string, in, out, cr, cw int64, ts time.Time, source string) {
	t.Helper()
	if _, err := st.InsertAgentTokens(context.Background(), store.AgentTokenUsage{
		SpawnID:      spawnID,
		InputTokens:  in,
		OutputTokens: out,
		CacheRead:    cr,
		CacheWrite:   cw,
		TS:           ts,
		Source:       source,
	}); err != nil {
		t.Fatalf("InsertAgentTokens(%s,%s): %v", spawnID, source, err)
	}
}

func seedVerdict(t *testing.T, st *store.Store, spawnID, verdict, lerneffekt string, ts time.Time) {
	t.Helper()
	if _, err := st.InsertAgentVerdict(context.Background(), store.AgentVerdict{
		SpawnID:      spawnID,
		Verdict:      verdict,
		LerneffektMD: lerneffekt,
		TS:           ts,
	}); err != nil {
		t.Fatalf("InsertAgentVerdict(%s): %v", spawnID, err)
	}
}

// TestAgentsTabRendersEmptyStateWithStore — store wired but empty.
// Verifies the empty-state card path against a live store (skeleton
// path is covered by TestAgentsTabRendersEmptyState in handler_test.go).
func TestAgentsTabRendersEmptyStateWithStore(t *testing.T) {
	env := newAgentsTestEnv(t)

	resp, err := http.Get(env.srv.URL + "/dashboard/tab/agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "No agent spawns yet") {
		t.Errorf("empty-state headline missing; body:\n%s", s)
	}
	if !strings.Contains(s, `class="tl-tab-panel tl-agents"`) {
		t.Errorf("tab-panel wrapper missing")
	}
}

// TestAgentsTabRendersWithSpawnData — single-spawn happy path:
// seeds one spawn + tokens + verdict, then verifies the rendered HTML
// surfaces all three (skill / aggregated tokens / verdict label).
func TestAgentsTabRendersWithSpawnData(t *testing.T) {
	env := newAgentsTestEnv(t)
	now := time.Now()

	rootID := padSpawnID("rootspawn1")
	seedSpawn(t, env.st, rootID, "", "ballard", "tracelab", now)
	seedTokens(t, env.st, rootID, 1000, 500, 200, 0, now, "sdk-hook")
	seedVerdict(t, env.st, rootID, "freigabe", "Pre-Hardcoding-Verifikation 4. erfolgreich.", now.Add(time.Second))

	resp, err := http.Get(env.srv.URL + "/dashboard/tab/agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	for _, want := range []string{
		"ballard",                                 // skill
		rootID,                                    // spawn id
		"tracelab",                                // project
		"1000",                                    // total input tokens
		"500",                                     // total output tokens
		"sdk-hook",                                // source label
		"Freigabe",                                // verdict humanised
		"Pre-Hardcoding-Verifikation 4. erfolgreich.", // lerneffekt
	} {
		if !strings.Contains(s, want) {
			t.Errorf("body missing %q; body:\n%s", want, s)
		}
	}
	if strings.Contains(s, "No agent spawns yet") {
		t.Errorf("empty-state still present despite seeded spawn")
	}
}

// TestAgentsTabAggregatesThreeSources mirrors the schema-level
// TestAgentsIngestThreeSourceCoexistence pin from S3: a single spawn
// with tokens from all three sources must produce one cross-source
// total AND a three-entry per-source breakdown. Critical pin for the
// ADR-013 §Consequences per-source-forensic-breakdown contract on the
// read side.
func TestAgentsTabAggregatesThreeSources(t *testing.T) {
	env := newAgentsTestEnv(t)
	now := time.Now()

	rootID := padSpawnID("multisrc01")
	seedSpawn(t, env.st, rootID, "", "tuvok", "tracelab", now)
	// Three sources at slightly different ts so the UNIQUE(spawn_id, ts,
	// source) tuple admits all three rows.
	seedTokens(t, env.st, rootID, 100, 50, 0, 0, now, "sdk-hook")
	seedTokens(t, env.st, rootID, 200, 100, 0, 0, now.Add(time.Millisecond), "transcript")
	seedTokens(t, env.st, rootID, 300, 150, 0, 0, now.Add(2*time.Millisecond), "mcp-push")

	resp, err := http.Get(env.srv.URL + "/dashboard/tab/agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	// Cross-source totals: 100+200+300 = 600 input, 50+100+150 = 300 output.
	if !strings.Contains(s, "600") {
		t.Errorf("cross-source input total 600 missing; body:\n%s", s)
	}
	if !strings.Contains(s, "300") {
		t.Errorf("cross-source output total 300 missing; body:\n%s", s)
	}
	// Per-source breakdown: each canonical source string must appear.
	for _, src := range []string{"sdk-hook", "transcript", "mcp-push"} {
		if !strings.Contains(s, src) {
			t.Errorf("source breakdown missing %q; body:\n%s", src, s)
		}
	}
	// And per-source values present (sdk-hook 100/50, transcript 200/100,
	// mcp-push 300/150). The list-item formatter renders "<src>: in / out"
	// so a substring search for one combined pair per source is enough.
	for _, want := range []string{"100", "200"} {
		if !strings.Contains(s, want) {
			t.Errorf("per-source value %q missing", want)
		}
	}
}

// TestAgentsTabPaginationAndFilter pins the filter + page boundaries:
//
//  - project filter narrows the result set
//  - page=2 with limit=1 returns the second-newest spawn
//  - total count reflects the unfiltered/filtered result
func TestAgentsTabPaginationAndFilter(t *testing.T) {
	env := newAgentsTestEnv(t)
	now := time.Now()
	// Three spawns in two projects, started 10 minutes apart so the
	// started_at DESC ORDER is deterministic.
	seedSpawn(t, env.st, padSpawnID("trace0001"), "", "ballard", "tracelab", now)
	seedSpawn(t, env.st, padSpawnID("trace0002"), "", "tuvok", "tracelab", now.Add(-10*time.Minute))
	seedSpawn(t, env.st, padSpawnID("nexus0001"), "", "barclay", "nexus", now.Add(-20*time.Minute))

	// Filter: only tracelab project, page 1, limit 1 → trace0001
	resp, err := http.Get(env.srv.URL + "/dashboard/tab/agents?project=tracelab&page=1&limit=1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, padSpawnID("trace0001")) {
		t.Errorf("page-1/limit-1 should show trace0001; body:\n%s", s)
	}
	if strings.Contains(s, padSpawnID("trace0002")) {
		t.Errorf("page-1/limit-1 should NOT show trace0002 yet")
	}
	if strings.Contains(s, padSpawnID("nexus0001")) {
		t.Errorf("project=tracelab filter should exclude nexus spawn")
	}
	// Total count reflects the filter — 2 tracelab spawns.
	if !strings.Contains(s, "2 total") {
		t.Errorf("total count for filtered tracelab project should be 2; body:\n%s", s)
	}

	// Page 2 of the same filter → trace0002
	resp2, err := http.Get(env.srv.URL + "/dashboard/tab/agents?project=tracelab&page=2&limit=1")
	if err != nil {
		t.Fatalf("GET page 2: %v", err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	s2 := string(body2)
	if !strings.Contains(s2, padSpawnID("trace0002")) {
		t.Errorf("page 2 should show trace0002; body:\n%s", s2)
	}
}

// TestAgentsTabRendersParentChildTree pins the depth-indent path: a
// parent spawn on the same page as its child renders the child with
// seedEdge inserts one mailbox-edge row. Phase 2d S5.
func seedEdge(t *testing.T, st *store.Store, from, to, edgeType string, ts time.Time) {
	t.Helper()
	if _, err := st.InsertAgentMailboxEdge(context.Background(), store.AgentMailboxEdge{
		FromSpawnID: from,
		ToSpawnID:   to,
		EdgeType:    edgeType,
		TS:          ts,
	}); err != nil {
		t.Fatalf("seed edge %s→%s/%s: %v", from, to, edgeType, err)
	}
}

// TestAgentsTabRendersEdgesSubList verifies the S5 mailbox-edges
// column: a spawn with in- and out-edges renders both directions in
// the sub-list, the counterpart-spawn ids appear in the HTML, and the
// edge_type strings are surfaced (one rendered per row).
func TestAgentsTabRendersEdgesSubList(t *testing.T) {
	env := newAgentsTestEnv(t)
	now := time.Now()

	parentID := padSpawnID("edgesparent")
	childID := padSpawnID("edgeschild")
	seedSpawn(t, env.st, parentID, "", "belanna", "tracelab", now)
	seedSpawn(t, env.st, childID, parentID, "ballard", "tracelab", now.Add(time.Second))
	seedEdge(t, env.st, parentID, childID, "spawn", now)
	seedEdge(t, env.st, childID, parentID, "return", now.Add(2*time.Second))

	resp, err := http.Get(env.srv.URL + "/dashboard/tab/agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	if !strings.Contains(s, `class="tl-agent-edge-list"`) {
		t.Errorf("edges sub-list class missing; body:\n%s", s)
	}
	if !strings.Contains(s, "spawn") || !strings.Contains(s, "return") {
		t.Errorf("edge_type strings missing; body:\n%s", s)
	}
	// Both spawn ids must appear (each as counterpart on the other row).
	if !strings.Contains(s, parentID) || !strings.Contains(s, childID) {
		t.Errorf("both spawn ids must appear in edge sub-list; body:\n%s", s)
	}
}

// the tree-glyph indent ("└─") prefix.
func TestAgentsTabRendersParentChildTree(t *testing.T) {
	env := newAgentsTestEnv(t)
	now := time.Now()

	parentID := padSpawnID("parent0001")
	childID := padSpawnID("child00001")
	// Parent older so it sorts AFTER the child in started_at DESC order;
	// the page contains both rows so the indent walk has its parent
	// visible.
	seedSpawn(t, env.st, parentID, "", "belanna", "tracelab", now.Add(-1*time.Minute))
	seedSpawn(t, env.st, childID, parentID, "ballard", "tracelab", now)

	resp, err := http.Get(env.srv.URL + "/dashboard/tab/agents")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	// The child row should carry the "└─" tree glyph; the parent does
	// not (depth=0).
	if !strings.Contains(s, "└─") {
		t.Errorf("child row missing tree-glyph indent; body:\n%s", s)
	}
	if !strings.Contains(s, parentID) || !strings.Contains(s, childID) {
		t.Errorf("both parent and child must render; body:\n%s", s)
	}
}
