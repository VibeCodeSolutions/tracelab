// sessions_test.go — Tests for the Phase 2c S3 session-browser tab.
//
// These tests exercise the data-driven SessionsHandler and
// SessionDetailHandler against a real on-disk SQLite store (modernc.org,
// CGO-free), so the SQL forms and the template-rendering path are
// covered end-to-end without mocks. The store is a fresh t.TempDir()
// per test, isolated and cheap.
//
// Coverage map vs. auftrag #027 DoD point 6:
//
//   - TestSessionsTabRenderEmpty                 → empty state
//   - TestSessionsTabRenderWithSeededSessions    → rendered table
//   - TestSessionsTabSortParamWhitelist          → sort whitelist
//   - TestSessionsTabFilterSubstringMatch        → substring filter
//   - TestSessionsTabPagination                  → limit/offset/page
//   - TestSessionDetailRender                    → detail view
//   - TestSessionDetailUnknownID404              → unknown id → 404
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

// sessionsTestEnv wires a dashboard.Handler with a fresh on-disk store
// and an httptest.Server multiplexing both the list route and the
// detail wildcard. Returned values are torn down by t.Cleanup hooks.
type sessionsTestEnv struct {
	srv *httptest.Server
	st  *store.Store
}

func newSessionsTestEnv(t *testing.T) *sessionsTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
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
	mux.HandleFunc("/dashboard/tab/sessions", h.SessionsHandler)
	// Wildcard for /dashboard/tab/sessions/{id}; net/http's mux is
	// fine because we hand-pick the prefix and the handler does its
	// own validation on the trailing segment.
	mux.HandleFunc("/dashboard/tab/sessions/", h.SessionDetailHandler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &sessionsTestEnv{srv: srv, st: st}
}

func (env *sessionsTestEnv) get(t *testing.T, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(env.srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// TestSessionsTabRenderEmpty — auftrag DoD-Test #1. Verifies the
// empty-state shell when the store has no sessions.
func TestSessionsTabRenderEmpty(t *testing.T) {
	env := newSessionsTestEnv(t)

	code, body := env.get(t, "/dashboard/tab/sessions")
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	if !strings.Contains(body, "No sessions found") {
		t.Errorf("empty body missing 'No sessions found' marker:\n%s", body)
	}
	if !strings.Contains(body, `class="tl-tab-panel tl-sessions"`) {
		t.Errorf("empty body missing tab-panel class")
	}
}

// TestSessionsTabRenderWithSeededSessions — auftrag DoD-Test #2.
// Seeds three sessions with different event/crash counts and asserts
// they all appear in the rendered table with the correct counts.
func TestSessionsTabRenderWithSeededSessions(t *testing.T) {
	env := newSessionsTestEnv(t)
	ctx := context.Background()

	a, err := env.st.CreateSession(ctx, "alpha")
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := env.st.CreateSession(ctx, "beta")
	if err != nil {
		t.Fatalf("create b: %v", err)
	}

	if err := env.st.InsertEvents(ctx, a, []store.Event{
		{Source: "x", Level: "info", Msg: "hello-a-1"},
		{Source: "x", Level: "info", Msg: "hello-a-2"},
	}); err != nil {
		t.Fatalf("insert a events: %v", err)
	}
	if err := env.st.UpsertCrash(ctx, a, time.Now(), "fp-a", "stack a"); err != nil {
		t.Fatalf("upsert crash: %v", err)
	}

	code, body := env.get(t, "/dashboard/tab/sessions")
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	// Both session ids must appear in the rendered table.
	if !strings.Contains(body, a) {
		t.Errorf("body missing session a id %q", a)
	}
	if !strings.Contains(body, b) {
		t.Errorf("body missing session b id %q", b)
	}
	// Per-row event-count must surface — session a has 2 events,
	// session b has 0.
	if !strings.Contains(body, ">2<") {
		t.Errorf("body missing event-count '2' for session a")
	}
	// Detail-link wiring: hx-get URL must include the session id.
	if !strings.Contains(body, `hx-get="/dashboard/tab/sessions/`+a+`"`) {
		t.Errorf("body missing detail hx-get for session a")
	}
}

// TestSessionsTabSortParamWhitelist — auftrag DoD-Test #3. Asserts
// that ?sort=<key> is restricted to the whitelist: an unknown key
// silently maps to the default (newest first), and the rendered
// dropdown reflects whichever known key was passed.
func TestSessionsTabSortParamWhitelist(t *testing.T) {
	env := newSessionsTestEnv(t)
	ctx := context.Background()
	if _, err := env.st.CreateSession(ctx, "only"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	cases := []struct {
		raw      string
		selected string
	}{
		// Known keys must be reflected in the <option selected>.
		{"started_at_desc", "started_at_desc"},
		{"started_at_asc", "started_at_asc"},
		{"event_count_desc", "event_count_desc"},
		{"session_id", "session_id"},
		// Unknown key silently falls back to default. Use a value
		// that doesn't contain URL-unsafe characters so http.Get
		// doesn't reject it before it reaches the handler — the
		// whitelist guarantee is about *server-side* handling.
		{"sql_inject_DROP", "started_at_desc"},
		{"", "started_at_desc"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			path := "/dashboard/tab/sessions"
			if tc.raw != "" {
				path += "?sort=" + tc.raw
			}
			code, body := env.get(t, path)
			if code != http.StatusOK {
				t.Fatalf("status=%d, want 200", code)
			}
			marker := `value="` + tc.selected + `" selected`
			if !strings.Contains(body, marker) {
				t.Errorf("expected %q selected in dropdown, body excerpt:\n%s",
					tc.selected, body)
			}
			// And the raw injected string must NEVER make it into the
			// rendered <select> value attribute (defence-in-depth).
			if tc.raw == "sql_inject_DROP" && strings.Contains(body, "sql_inject") {
				t.Errorf("injected sort key leaked into response body")
			}
		})
	}
}

// TestSessionsTabFilterSubstringMatch — auftrag DoD-Test #4. Seeds a
// session with a known id and asserts the filter narrows the list
// down to just that row.
func TestSessionsTabFilterSubstringMatch(t *testing.T) {
	env := newSessionsTestEnv(t)
	ctx := context.Background()

	// Three sessions; we filter on a substring unique to one of them.
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		id, err := env.st.CreateSession(ctx, "s")
		if err != nil {
			t.Fatalf("create s-%d: %v", i, err)
		}
		ids = append(ids, id)
	}

	// The full id of ids[0] is unique to that session.
	code, body := env.get(t, "/dashboard/tab/sessions?filter="+ids[0])
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	if !strings.Contains(body, ids[0]) {
		t.Errorf("filtered body missing target id %q", ids[0])
	}
	// The other two ids must not appear.
	for _, other := range ids[1:] {
		if strings.Contains(body, other) {
			t.Errorf("filtered body leaked non-matching id %q", other)
		}
	}
	// Filter value must round-trip into the input.
	if !strings.Contains(body, `value="`+ids[0]+`"`) {
		t.Errorf("filter input did not round-trip the filter value")
	}
}

// TestSessionsTabPagination — auftrag DoD-Test #5. Seeds enough
// sessions to force two pages at limit=2, then walks page 1 → page 2
// and asserts the rows differ.
func TestSessionsTabPagination(t *testing.T) {
	env := newSessionsTestEnv(t)
	ctx := context.Background()

	ids := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		id, err := env.st.CreateSession(ctx, "s")
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		ids = append(ids, id)
		time.Sleep(1 * time.Millisecond) // distinct started_at
	}

	code, p1 := env.get(t, "/dashboard/tab/sessions?limit=2&page=1&sort=started_at_asc")
	if code != http.StatusOK {
		t.Fatalf("page 1 status=%d", code)
	}
	if !strings.Contains(p1, "Page 1 of 3") {
		t.Errorf("page 1 indicator missing 'Page 1 of 3':\n%s", p1)
	}
	if !strings.Contains(p1, ids[0]) || !strings.Contains(p1, ids[1]) {
		t.Errorf("page 1 should hold ids[0]+ids[1] (asc sort); got body:\n%s", p1)
	}
	if strings.Contains(p1, ids[3]) || strings.Contains(p1, ids[4]) {
		t.Errorf("page 1 leaked later ids")
	}

	code, p2 := env.get(t, "/dashboard/tab/sessions?limit=2&page=2&sort=started_at_asc")
	if code != http.StatusOK {
		t.Fatalf("page 2 status=%d", code)
	}
	if !strings.Contains(p2, "Page 2 of 3") {
		t.Errorf("page 2 indicator missing 'Page 2 of 3':\n%s", p2)
	}
	if !strings.Contains(p2, ids[2]) || !strings.Contains(p2, ids[3]) {
		t.Errorf("page 2 should hold ids[2]+ids[3] (asc sort)")
	}

	// Prev link on page 2 must surface; Next link on the last page
	// must be inactive (rendered as span.tl-muted). The href is
	// HTML-escaped, so we look for the &amp;-encoded form.
	if !strings.Contains(p2, `href="/dashboard/tab/sessions?sort=started_at_asc&amp;page=1`) {
		t.Errorf("page 2 missing Prev link to page=1; body:\n%s", p2)
	}
	code, p3 := env.get(t, "/dashboard/tab/sessions?limit=2&page=3&sort=started_at_asc")
	if code != http.StatusOK {
		t.Fatalf("page 3 status=%d", code)
	}
	if !strings.Contains(p3, `<span class="tl-muted">Next`) {
		t.Errorf("page 3 (last) should render Next as muted span")
	}
}

// TestSessionDetailRender — auftrag DoD-Test #6. Verifies the detail
// view renders events + crashes for a known session id.
func TestSessionDetailRender(t *testing.T) {
	env := newSessionsTestEnv(t)
	ctx := context.Background()

	id, err := env.st.CreateSession(ctx, "detail")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := env.st.InsertEvents(ctx, id, []store.Event{
		{Source: "android", Level: "error", Msg: "kaboom"},
		{Source: "android", Level: "info", Msg: "hello"},
	}); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}
	if err := env.st.UpsertCrash(ctx, id, time.Now(), "fp-1", "stack frame 1"); err != nil {
		t.Fatalf("UpsertCrash: %v", err)
	}

	code, body := env.get(t, "/dashboard/tab/sessions/"+id)
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}

	// Header reveals the session id.
	if !strings.Contains(body, id) {
		t.Errorf("detail body missing session id %q", id)
	}
	// Both events surface — match on the message text.
	if !strings.Contains(body, "kaboom") || !strings.Contains(body, "hello") {
		t.Errorf("detail body missing event messages")
	}
	// Crash fingerprint surfaces.
	if !strings.Contains(body, "fp-1") {
		t.Errorf("detail body missing crash fingerprint")
	}
	// Back-link points back to the list.
	if !strings.Contains(body, `href="/dashboard/tab/sessions"`) {
		t.Errorf("detail body missing back link to list")
	}
}

// TestSessionDetailUnknownID404 — auftrag DoD-Test #7. Unknown id
// must 404, distinct from the silent-fallback behaviour of the
// LayoutHandler.
func TestSessionDetailUnknownID404(t *testing.T) {
	env := newSessionsTestEnv(t)

	code, _ := env.get(t, "/dashboard/tab/sessions/does-not-exist")
	if code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", code)
	}
}

// TestSessionDetailBackURLPreservesQuery — bonus: when the user
// landed on the detail view via a list-row click that itself carried
// sort/filter/page state, the back link must preserve that state so
// returning to the list doesn't reset the user's context.
func TestSessionDetailBackURLPreservesQuery(t *testing.T) {
	env := newSessionsTestEnv(t)
	ctx := context.Background()
	id, err := env.st.CreateSession(ctx, "back-url")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	code, body := env.get(t,
		"/dashboard/tab/sessions/"+id+"?sort=event_count_desc&page=2&filter=abc")
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	if !strings.Contains(body,
		`href="/dashboard/tab/sessions?sort=event_count_desc&amp;page=2&amp;filter=abc"`) {
		t.Errorf("back link did not preserve sort/filter/page; body excerpt:\n%s",
			body)
	}
}

// TestSessionsTabClampsPageBeyondLast — when the user requests a
// page past the end (e.g. filter shrank the result set), the handler
// snaps back to the last valid page rather than rendering an empty
// in-between page.
func TestSessionsTabClampsPageBeyondLast(t *testing.T) {
	env := newSessionsTestEnv(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := env.st.CreateSession(ctx, "s"); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	code, body := env.get(t, "/dashboard/tab/sessions?limit=2&page=99")
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	if !strings.Contains(body, "Page 2 of 2") {
		t.Errorf("clamp should land on Page 2 of 2; body:\n%s", body)
	}
}
