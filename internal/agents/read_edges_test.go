// read_edges_test.go — Phase 2d S5 acceptance tests for GET /agents/edges.
//
// Coverage:
//
//   - TestAgentsReadEdgesEmpty           → spawn without edges → empty in/out
//   - TestAgentsReadEdgesInOut           → seeded in-edge and out-edge split
//   - TestAgentsReadEdgesMissingParam    → 400 on missing spawn_id
//   - TestAgentsReadEdgesInvalidFormat   → 400 on rogue spawn_id chars
//   - TestAgentsReadEdgesBearerAuth      → 401 without bearer
//   - TestAgentsReadEdgesAllEdgeTypes    → all four edge_type enum values

package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// seedEdgeRow inserts a mailbox-edge through the public store API.
func seedEdgeRow(t *testing.T, st *store.Store, from, to, edgeType string, ts time.Time) {
	t.Helper()
	_, err := st.InsertAgentMailboxEdge(context.Background(), store.AgentMailboxEdge{
		FromSpawnID: from,
		ToSpawnID:   to,
		EdgeType:    edgeType,
		TS:          ts,
	})
	if err != nil {
		t.Fatalf("InsertAgentMailboxEdge(%s→%s/%s): %v", from, to, edgeType, err)
	}
}

func TestAgentsReadEdgesEmpty(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	spawnID := "lonelyaaaaaaaaaaaaaaaaaaaa"
	seedSpawnRow(t, env.st, spawnID, "", "ballard", "tracelab", now)

	resp := bearerGET(t, env.srv, "/agents/edges?spawn_id="+spawnID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body edgesResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SpawnID != spawnID {
		t.Errorf("spawn_id=%q, want %q", body.SpawnID, spawnID)
	}
	if len(body.In) != 0 {
		t.Errorf("in=%v, want empty", body.In)
	}
	if len(body.Out) != 0 {
		t.Errorf("out=%v, want empty", body.Out)
	}
	// The wire shape MUST emit empty arrays, not null — the decode above
	// would tolerate null but the dashboard JS reads .length without a
	// nil-guard. Re-encode to confirm.
	raw, _ := json.Marshal(body)
	if !contains(string(raw), `"in":[]`) || !contains(string(raw), `"out":[]`) {
		t.Errorf("wire shape lacks empty-array marker: %s", string(raw))
	}
}

func TestAgentsReadEdgesInOut(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	parent := "parentaaaaaaaaaaaaaaaaaaaa"
	child := "childaaaaaaaaaaaaaaaaaaaaa"
	sibling := "siblingaaaaaaaaaaaaaaaaaaa"
	seedSpawnRow(t, env.st, parent, "", "belanna", "tracelab", now)
	seedSpawnRow(t, env.st, child, parent, "ballard", "tracelab", now)
	seedSpawnRow(t, env.st, sibling, parent, "tuvok", "tracelab", now)
	seedEdgeRow(t, env.st, parent, child, "spawn", now)
	seedEdgeRow(t, env.st, parent, sibling, "spawn", now.Add(time.Second))
	seedEdgeRow(t, env.st, child, parent, "return", now.Add(2*time.Second))

	resp := bearerGET(t, env.srv, "/agents/edges?spawn_id="+parent)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body edgesResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.In) != 1 {
		t.Fatalf("in=%d, want 1", len(body.In))
	}
	if body.In[0].FromSpawnID != child || body.In[0].EdgeType != "return" {
		t.Errorf("in[0]=%+v", body.In[0])
	}
	if len(body.Out) != 2 {
		t.Fatalf("out=%d, want 2", len(body.Out))
	}
	if body.Out[0].ToSpawnID != child || body.Out[1].ToSpawnID != sibling {
		t.Errorf("out ordering wrong: %+v", body.Out)
	}
}

func TestAgentsReadEdgesMissingParam(t *testing.T) {
	env := newReadEnv(t)
	resp := bearerGET(t, env.srv, "/agents/edges")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestAgentsReadEdgesInvalidFormat(t *testing.T) {
	env := newReadEnv(t)
	resp := bearerGET(t, env.srv, "/agents/edges?spawn_id=../etc/passwd")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestAgentsReadEdgesBearerAuth(t *testing.T) {
	env := newReadEnv(t)
	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/agents/edges?spawn_id=x", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", resp.StatusCode)
	}
}

func TestAgentsReadEdgesAllEdgeTypes(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	a := "aaaaaaaaaaaaaaaaaaaaaaaaaa"
	b := "bbbbbbbbbbbbbbbbbbbbbbbbbb"
	seedSpawnRow(t, env.st, a, "", "x", "p", now)
	seedSpawnRow(t, env.st, b, "", "y", "p", now)

	for i, typ := range []string{"spawn", "return", "escalate", "delegate"} {
		seedEdgeRow(t, env.st, a, b, typ, now.Add(time.Duration(i)*time.Second))
	}
	resp := bearerGET(t, env.srv, "/agents/edges?spawn_id="+a)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body edgesResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Out) != 4 {
		t.Fatalf("out=%d, want 4", len(body.Out))
	}
	wantOrder := []string{"spawn", "return", "escalate", "delegate"}
	for i, want := range wantOrder {
		if body.Out[i].EdgeType != want {
			t.Errorf("out[%d].edge_type=%q, want %q", i, body.Out[i].EdgeType, want)
		}
	}
}

// contains is a tiny local substring check so the test file doesn't pull
// in strings for one call site (matches the package's "minimal-deps"
// posture in transcript.go).
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
