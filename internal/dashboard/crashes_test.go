// crashes_test.go — Tests for the Phase 2c S4 crash-inspector tab.
//
// Mirrors the sessions_test.go pattern: a real on-disk SQLite store
// (modernc.org, CGO-free) per test, isolated via t.TempDir(), so the
// SQL forms and the template-rendering path are covered end-to-end
// without mocks.
//
// Coverage map vs. auftrag #028 DoD point 6:
//
//   - TestCrashesTabRenderEmpty                  → empty state
//   - TestCrashesTabRenderWithSeededCrashes      → rendered table + dedup
//   - TestCrashesTabSessionFilterForwarding      → ?session=… narrows list
//   - TestCrashesTabSortParamWhitelist           → sort whitelist
//   - TestCrashDetailRender                      → detail view with stack
//   - TestCrashDetailUnknownID404                → unknown id → 404
//   - TestCrashDetailBackURLPreservesQuery       → back-link state
//   - TestCrashDetailEmptyStacktraceGraceful     → graceful empty stack
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

// crashesTestEnv wires a dashboard.Handler with a fresh on-disk store
// and an httptest.Server multiplexing both the list route and the
// detail wildcard. Torn down by t.Cleanup hooks.
type crashesTestEnv struct {
	srv *httptest.Server
	st  *store.Store
}

func newCrashesTestEnv(t *testing.T) *crashesTestEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "crashes.db")
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
	mux.HandleFunc("/dashboard/tab/crashes", h.CrashesHandler)
	mux.HandleFunc("/dashboard/tab/crashes/", h.CrashDetailHandler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &crashesTestEnv{srv: srv, st: st}
}

func (env *crashesTestEnv) get(t *testing.T, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(env.srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// seedSessionWithCrash creates a session, upserts a crash, and returns
// (sessionID, crashID). The crashID is recovered via the store's
// CrashesBySession (S4-additive functions are tested elsewhere; here
// the helper only needs an id round-trip).
func seedSessionWithCrash(t *testing.T, env *crashesTestEnv, label, fingerprint, stack string) (string, int64) {
	t.Helper()
	ctx := context.Background()
	sid, err := env.st.CreateSession(ctx, label)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := env.st.UpsertCrash(ctx, sid, time.Now(), fingerprint, stack); err != nil {
		t.Fatalf("UpsertCrash: %v", err)
	}
	rows, err := env.st.CrashesBySession(ctx, sid, 10)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("seeded crash not found via CrashesBySession")
	}
	return sid, rows[0].ID
}

// TestCrashesTabRenderEmpty — DoD-Test #1. Verifies the empty-state
// shell when the store has no crashes.
func TestCrashesTabRenderEmpty(t *testing.T) {
	env := newCrashesTestEnv(t)

	code, body := env.get(t, "/dashboard/tab/crashes")
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	if !strings.Contains(body, "No crashes found") {
		t.Errorf("empty body missing 'No crashes found' marker:\n%s", body)
	}
	if !strings.Contains(body, `class="tl-tab-panel tl-crashes"`) {
		t.Errorf("empty body missing tab-panel class")
	}
	// The session-filter dropdown must always have at least the
	// "All sessions" sentinel option.
	if !strings.Contains(body, "All sessions") {
		t.Errorf("empty body missing All-sessions sentinel")
	}
}

// TestCrashesTabRenderWithSeededCrashes — DoD-Test #2. Seeds two
// sessions with multiple fingerprints each and asserts the
// deduplicated rows appear with their counts and top-frames preview.
func TestCrashesTabRenderWithSeededCrashes(t *testing.T) {
	env := newCrashesTestEnv(t)
	ctx := context.Background()

	sA, err := env.st.CreateSession(ctx, "alpha")
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	sB, err := env.st.CreateSession(ctx, "beta")
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}

	// Two distinct fingerprints in session A; one of them seen twice
	// to verify the count surfaces.
	if err := env.st.UpsertCrash(ctx, sA, time.Now(), "fp-a1",
		"at com.example.Foo.bar(Foo.java:42)\nat com.example.App.main(App.java:7)"); err != nil {
		t.Fatalf("upsert a1: %v", err)
	}
	if err := env.st.UpsertCrash(ctx, sA, time.Now(), "fp-a1",
		"at com.example.Foo.bar(Foo.java:42)\nat com.example.App.main(App.java:7)"); err != nil {
		t.Fatalf("upsert a1 dup: %v", err)
	}
	if err := env.st.UpsertCrash(ctx, sA, time.Now(), "fp-a2",
		"at com.example.Bar.baz(Bar.java:9)"); err != nil {
		t.Fatalf("upsert a2: %v", err)
	}
	// One fingerprint in session B — distinct from A.
	if err := env.st.UpsertCrash(ctx, sB, time.Now(), "fp-b1",
		"at com.example.Quux.tick(Quux.java:1)"); err != nil {
		t.Fatalf("upsert b1: %v", err)
	}

	code, body := env.get(t, "/dashboard/tab/crashes")
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	// All three deduplicated fingerprints must surface.
	for _, fp := range []string{"fp-a1", "fp-a2", "fp-b1"} {
		if !strings.Contains(body, fp) {
			t.Errorf("body missing fingerprint %q", fp)
		}
	}
	// Both session ids must surface in the table rows.
	if !strings.Contains(body, sA) || !strings.Contains(body, sB) {
		t.Errorf("body missing session ids")
	}
	// fp-a1 was upserted twice → count column must show 2.
	if !strings.Contains(body, ">2<") {
		t.Errorf("body missing count=2 for duplicate fingerprint")
	}
	// Top-frames preview must surface — match on the actual frame text.
	if !strings.Contains(body, "Foo.java:42") {
		t.Errorf("body missing top-frames preview from fp-a1")
	}
	// Both session ids must surface in the session-filter dropdown.
	if !strings.Contains(body, `<option value="`+sA+`"`) {
		t.Errorf("session dropdown missing sA option")
	}
	if !strings.Contains(body, `<option value="`+sB+`"`) {
		t.Errorf("session dropdown missing sB option")
	}
	// Detail hx-get must wire to the crash id (which is numeric).
	if !strings.Contains(body, `hx-get="/dashboard/tab/crashes/`) {
		t.Errorf("body missing detail hx-get prefix")
	}
}

// TestCrashesTabSessionFilterForwarding — DoD-Test #3. Asserts that
// passing ?session=<id> restricts the rendered table to that session's
// crashes only.
func TestCrashesTabSessionFilterForwarding(t *testing.T) {
	env := newCrashesTestEnv(t)
	ctx := context.Background()

	sA, err := env.st.CreateSession(ctx, "alpha")
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	sB, err := env.st.CreateSession(ctx, "beta")
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}
	if err := env.st.UpsertCrash(ctx, sA, time.Now(), "fp-only-a", "at A.a(A.java:1)"); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := env.st.UpsertCrash(ctx, sB, time.Now(), "fp-only-b", "at B.b(B.java:1)"); err != nil {
		t.Fatalf("upsert b: %v", err)
	}

	code, body := env.get(t, "/dashboard/tab/crashes?session="+sA)
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200", code)
	}
	if !strings.Contains(body, "fp-only-a") {
		t.Errorf("filtered body missing fp-only-a:\n%s", body)
	}
	if strings.Contains(body, "fp-only-b") {
		t.Errorf("filtered body leaked fp-only-b — session filter not applied")
	}
	// The selected dropdown option must reflect the active filter.
	marker := `value="` + sA + `" selected`
	if !strings.Contains(body, marker) {
		t.Errorf("session dropdown didn't mark %q as selected", sA)
	}
}

// TestCrashesTabSortParamWhitelist — DoD-Test #4. Unknown ?sort= keys
// silently fall back to the default; known keys round-trip into the
// dropdown's selected option.
func TestCrashesTabSortParamWhitelist(t *testing.T) {
	env := newCrashesTestEnv(t)
	ctx := context.Background()
	sid, err := env.st.CreateSession(ctx, "s")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := env.st.UpsertCrash(ctx, sid, time.Now(), "fp", "at X.y(X.java:1)"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	cases := []struct {
		raw      string
		selected string
	}{
		{"ts_desc", "ts_desc"},
		{"ts_asc", "ts_asc"},
		{"count_desc", "count_desc"},
		{"fingerprint_asc", "fingerprint_asc"},
		// Unknown sort key falls back silently to the default.
		{"sql_inject_DROP", "ts_desc"},
		{"", "ts_desc"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			path := "/dashboard/tab/crashes"
			if tc.raw != "" {
				path += "?sort=" + tc.raw
			}
			code, body := env.get(t, path)
			if code != http.StatusOK {
				t.Fatalf("status=%d, want 200", code)
			}
			marker := `value="` + tc.selected + `" selected`
			if !strings.Contains(body, marker) {
				t.Errorf("expected %q selected in dropdown; body excerpt:\n%s",
					tc.selected, body)
			}
			if tc.raw == "sql_inject_DROP" && strings.Contains(body, "sql_inject") {
				t.Errorf("injected sort key leaked into rendered HTML")
			}
		})
	}
}

// TestCrashDetailRender — DoD-Test #5. Verifies the detail view
// renders fingerprint, session id, count and the full stacktrace for
// a known crash id.
func TestCrashDetailRender(t *testing.T) {
	env := newCrashesTestEnv(t)

	stack := "Exception in thread \"main\" java.lang.NullPointerException\n" +
		"\tat com.example.Foo.bar(Foo.java:42)\n" +
		"\tat com.example.Foo.baz(Foo.java:99)\n" +
		"\tat com.example.App.main(App.java:7)"
	sid, cid := seedSessionWithCrash(t, env, "detail", "fp-detail", stack)

	code, body := env.get(t, "/dashboard/tab/crashes/"+itoa(cid))
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", code, body)
	}
	// Header surfaces fingerprint + session + count.
	if !strings.Contains(body, "fp-detail") {
		t.Errorf("detail body missing fingerprint")
	}
	if !strings.Contains(body, sid) {
		t.Errorf("detail body missing session id %q", sid)
	}
	// Full stacktrace must surface — match on a deep frame the
	// list-view preview doesn't include (App.java:7 is the third
	// frame, beyond the preview's top-3).
	if !strings.Contains(body, "App.java:7") {
		t.Errorf("detail body missing deep stacktrace frame")
	}
	// Back-link points to the list.
	if !strings.Contains(body, `href="/dashboard/tab/crashes"`) {
		t.Errorf("detail body missing back link to list")
	}
}

// TestCrashDetailUnknownID404 — DoD-Test #6. Unknown id must 404.
func TestCrashDetailUnknownID404(t *testing.T) {
	env := newCrashesTestEnv(t)

	// Numeric id that cannot exist.
	code, _ := env.get(t, "/dashboard/tab/crashes/99999")
	if code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 for unknown id", code)
	}
	// Non-numeric id is also a 404 (the handler rejects parse failures).
	code, _ = env.get(t, "/dashboard/tab/crashes/not-a-number")
	if code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 for non-numeric id", code)
	}
}

// TestCrashDetailBackURLPreservesQuery — DoD-Test #7 (optional). The
// back link must preserve the sort/session/page state the user came
// from, so returning to the list doesn't reset their context.
func TestCrashDetailBackURLPreservesQuery(t *testing.T) {
	env := newCrashesTestEnv(t)
	_, cid := seedSessionWithCrash(t, env, "back-url", "fp", "at X.y(X.java:1)")

	code, body := env.get(t,
		"/dashboard/tab/crashes/"+itoa(cid)+"?sort=count_desc&page=2&session=abc")
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", code, body)
	}
	if !strings.Contains(body,
		`href="/dashboard/tab/crashes?sort=count_desc&amp;page=2&amp;session=abc"`) {
		t.Errorf("back link did not preserve sort/page/session; body excerpt:\n%s", body)
	}
}

// TestCrashDetailEmptyStacktraceGraceful — DoD-Test #8 (optional).
// A crash row with an empty stacktrace string must render the detail
// view with a graceful "No stacktrace recorded" message rather than
// blank pre-block. UpsertCrash accepts empty stacktrace strings (only
// the fingerprint is required), so this path is reachable in practice.
func TestCrashDetailEmptyStacktraceGraceful(t *testing.T) {
	env := newCrashesTestEnv(t)
	_, cid := seedSessionWithCrash(t, env, "empty", "fp-empty", "")

	code, body := env.get(t, "/dashboard/tab/crashes/"+itoa(cid))
	if code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", code, body)
	}
	if !strings.Contains(body, "No stacktrace recorded") {
		t.Errorf("empty-stacktrace detail missing graceful empty-state:\n%s", body)
	}
}

// TestCrashesTabPagination — bonus: seed enough rows to force two
// pages at limit=2 and verify page indicator + row distinctness.
func TestCrashesTabPagination(t *testing.T) {
	env := newCrashesTestEnv(t)
	ctx := context.Background()
	sid, err := env.st.CreateSession(ctx, "page")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Five distinct fingerprints, each with a slight ts offset so the
	// ts_asc order is stable across rows.
	for i := 0; i < 5; i++ {
		fp := "fp-" + itoa(int64(i))
		if err := env.st.UpsertCrash(ctx, sid, time.Now(),
			fp, "at Frame"+itoa(int64(i))+".m(Frame"+itoa(int64(i))+".java:1)"); err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	code, p1 := env.get(t, "/dashboard/tab/crashes?limit=2&page=1&sort=ts_asc")
	if code != http.StatusOK {
		t.Fatalf("page 1 status=%d", code)
	}
	if !strings.Contains(p1, "Page 1 of 3") {
		t.Errorf("page 1 indicator missing 'Page 1 of 3':\n%s", p1)
	}
	if !strings.Contains(p1, "fp-0") || !strings.Contains(p1, "fp-1") {
		t.Errorf("page 1 should hold fp-0+fp-1 (asc sort); got body:\n%s", p1)
	}
	if strings.Contains(p1, "fp-3") || strings.Contains(p1, "fp-4") {
		t.Errorf("page 1 leaked later fingerprints")
	}

	code, p2 := env.get(t, "/dashboard/tab/crashes?limit=2&page=2&sort=ts_asc")
	if code != http.StatusOK {
		t.Fatalf("page 2 status=%d", code)
	}
	if !strings.Contains(p2, "Page 2 of 3") {
		t.Errorf("page 2 indicator missing 'Page 2 of 3':\n%s", p2)
	}
	if !strings.Contains(p2, "fp-2") || !strings.Contains(p2, "fp-3") {
		t.Errorf("page 2 should hold fp-2+fp-3 (asc sort)")
	}
	// Last page renders Next as a muted span.
	code, p3 := env.get(t, "/dashboard/tab/crashes?limit=2&page=3&sort=ts_asc")
	if code != http.StatusOK {
		t.Fatalf("page 3 status=%d", code)
	}
	if !strings.Contains(p3, `<span class="tl-muted">Next`) {
		t.Errorf("page 3 (last) should render Next as muted span")
	}
}

// itoa is a tiny helper local to this test file — strconv.FormatInt
// inlined to keep the imports minimal. Used to render crash ids into
// URL paths.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
