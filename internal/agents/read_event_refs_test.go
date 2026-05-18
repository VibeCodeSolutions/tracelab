// read_event_refs_test.go — Phase 2d S5-Tail acceptance tests for
// GET /agents/event_refs (ADR-014 Accepted, Option B).
//
// Coverage:
//
//   - TestAgentsReadEventRefsEmpty         → spawn without refs → empty array
//   - TestAgentsReadEventRefsMultiRef      → multiple refs, ts ASC ordering
//   - TestAgentsReadEventRefsMissingParam  → 400 on missing spawn_id
//   - TestAgentsReadEventRefsInvalidFormat → 400 on rogue spawn_id chars
//   - TestAgentsReadEventRefsBearerAuth    → 401 without bearer
//   - TestAgentsReadEventRefsAllRefTypes   → all three ref_type enum values

package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// seedEventInSession seeds a session + event via the public store API
// and returns the AUTOINCREMENT events.id (FK target for event_refs).
func seedEventInSession(t *testing.T, st *store.Store, label string) int64 {
	t.Helper()
	ctx := context.Background()
	sessionID, err := st.CreateSession(ctx, label)
	if err != nil {
		t.Fatalf("CreateSession(%s): %v", label, err)
	}
	if err := st.InsertEvents(ctx, sessionID, []store.Event{{
		SessionID: sessionID,
		TS:        time.Unix(1700000000, 0),
		Source:    "app",
		Level:     "info",
		Msg:       label,
	}}); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}
	var id int64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT id FROM events WHERE session_id = ? ORDER BY id DESC LIMIT 1`,
		sessionID,
	).Scan(&id); err != nil {
		t.Fatalf("query event id: %v", err)
	}
	return id
}

// seedEventRefRow inserts a single agent_event_refs row via the public
// store API.
func seedEventRefRow(t *testing.T, st *store.Store, spawnID string, eventID int64, refType string, ts time.Time) {
	t.Helper()
	if _, err := st.InsertAgentEventRef(context.Background(), store.AgentEventRef{
		SpawnID: spawnID,
		EventID: eventID,
		RefType: refType,
		TS:      ts,
	}); err != nil {
		t.Fatalf("InsertAgentEventRef(%s→%d/%s): %v", spawnID, eventID, refType, err)
	}
}

func TestAgentsReadEventRefsEmpty(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	spawnID := "lonelyref0000000000000000a"
	seedSpawnRow(t, env.st, spawnID, "", "ballard", "tracelab", now)

	resp := bearerGET(t, env.srv, "/agents/event_refs?spawn_id="+spawnID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body eventRefsResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SpawnID != spawnID {
		t.Errorf("spawn_id=%q, want %q", body.SpawnID, spawnID)
	}
	if len(body.EventRefs) != 0 {
		t.Errorf("event_refs=%v, want empty", body.EventRefs)
	}
	// Wire shape MUST emit an empty array, not null.
	raw, _ := json.Marshal(body)
	if !contains(string(raw), `"event_refs":[]`) {
		t.Errorf("wire shape lacks empty-array marker: %s", string(raw))
	}
}

func TestAgentsReadEventRefsMultiRef(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)

	spawnID := "multispawn1111111111111111"
	seedSpawnRow(t, env.st, spawnID, "", "ballard", "tracelab", now)
	e1 := seedEventInSession(t, env.st, "ref-1")
	e2 := seedEventInSession(t, env.st, "ref-2")

	// Insert in mixed temporal order to exercise ts ASC, id ASC ordering.
	seedEventRefRow(t, env.st, spawnID, e2, "observed", now.Add(2*time.Second))
	seedEventRefRow(t, env.st, spawnID, e1, "context", now)
	seedEventRefRow(t, env.st, spawnID, e2, "caused-by", now.Add(time.Second))

	resp := bearerGET(t, env.srv, "/agents/event_refs?spawn_id="+spawnID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body eventRefsResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.EventRefs) != 3 {
		t.Fatalf("event_refs=%d, want 3", len(body.EventRefs))
	}
	// Expect ts ASC: e1/context (now), e2/caused-by (now+1s), e2/observed (now+2s).
	want := []struct {
		eventID int64
		refType string
	}{
		{e1, "context"},
		{e2, "caused-by"},
		{e2, "observed"},
	}
	for i, w := range want {
		if body.EventRefs[i].EventID != w.eventID {
			t.Errorf("[%d].event_id=%d, want %d", i, body.EventRefs[i].EventID, w.eventID)
		}
		if body.EventRefs[i].RefType != w.refType {
			t.Errorf("[%d].ref_type=%q, want %q", i, body.EventRefs[i].RefType, w.refType)
		}
		if body.EventRefs[i].SpawnID != spawnID {
			t.Errorf("[%d].spawn_id=%q, want %q", i, body.EventRefs[i].SpawnID, spawnID)
		}
	}
}

func TestAgentsReadEventRefsMissingParam(t *testing.T) {
	env := newReadEnv(t)
	resp := bearerGET(t, env.srv, "/agents/event_refs")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestAgentsReadEventRefsInvalidFormat(t *testing.T) {
	env := newReadEnv(t)
	resp := bearerGET(t, env.srv, "/agents/event_refs?spawn_id=../etc/passwd")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", resp.StatusCode)
	}
}

func TestAgentsReadEventRefsBearerAuth(t *testing.T) {
	env := newReadEnv(t)
	req, _ := http.NewRequest(http.MethodGet, env.srv.URL+"/agents/event_refs?spawn_id=x", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", resp.StatusCode)
	}
}

func TestAgentsReadEventRefsAllRefTypes(t *testing.T) {
	env := newReadEnv(t)
	now := time.Unix(1700000000, 0)
	spawnID := "alltypes0000000000000000ab"
	seedSpawnRow(t, env.st, spawnID, "", "ballard", "tracelab", now)
	eventID := seedEventInSession(t, env.st, "alltypes")

	for i, typ := range []string{"observed", "context", "caused-by"} {
		seedEventRefRow(t, env.st, spawnID, eventID, typ, now.Add(time.Duration(i)*time.Second))
	}
	resp := bearerGET(t, env.srv, "/agents/event_refs?spawn_id="+spawnID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body eventRefsResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.EventRefs) != 3 {
		t.Fatalf("event_refs=%d, want 3", len(body.EventRefs))
	}
	wantOrder := []string{"observed", "context", "caused-by"}
	for i, want := range wantOrder {
		if body.EventRefs[i].RefType != want {
			t.Errorf("[%d].ref_type=%q, want %q", i, body.EventRefs[i].RefType, want)
		}
	}
}
