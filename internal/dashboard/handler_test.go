package dashboard_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VibeCodeSolutions/tracelab/internal/dashboard"
)

const testVersion = "0.0.0-test"

func newHandler(t *testing.T) *dashboard.Handler {
	t.Helper()
	// Skeleton-only tests pass nil store; the static-shell render
	// path (tabTpl + LayoutHandler fallback) covers the sessions
	// slug via the legacy placeholder until a Store is wired in.
	h, err := dashboard.NewHandler(testVersion, nil, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

// TestNewHandlerParsesEmbeddedTemplates pins the start-up posture:
// embedding is intact and every templates/*.gohtml parses. Regression
// guard for an accidental rename or a missing-tab template.
func TestNewHandlerParsesEmbeddedTemplates(t *testing.T) {
	h := newHandler(t)
	if h == nil {
		t.Fatal("NewHandler returned nil without error")
	}
}

// TestLayoutHandler_RendersAllTabsAndDefaultBody verifies that GET
// /dashboard returns the full layout, with every tab label visible in
// the navigation, the live-tail tab marked active by default, and the
// active-tab placeholder body present.
func TestLayoutHandler_RendersAllTabsAndDefaultBody(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.LayoutHandler))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/dashboard")
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type=%q, want text/html", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyStr := string(body)

	// All four tab labels must appear in the navigation.
	for _, label := range []string{"Live-Tail", "Sessions", "Crashes", "Agents"} {
		if !strings.Contains(bodyStr, label) {
			t.Errorf("layout missing tab label %q", label)
		}
	}
	// Default tab is live-tail; its placeholder body must be rendered.
	if !strings.Contains(bodyStr, `id="live-tail-output"`) {
		t.Errorf("default body missing live-tail SSE output target")
	}
	// Active class must mark the live-tail anchor.
	if !strings.Contains(bodyStr, `data-tab="live-tail"`) ||
		!strings.Contains(bodyStr, "tl-tab-active") {
		t.Errorf("layout did not mark default tab active")
	}
	// htmx + CSS asset links present.
	if !strings.Contains(bodyStr, "/dashboard/static/htmx.min.js") {
		t.Errorf("layout missing htmx asset link")
	}
	if !strings.Contains(bodyStr, "/dashboard/static/dashboard.css") {
		t.Errorf("layout missing dashboard.css link")
	}
	// Version surfaces in the header.
	if !strings.Contains(bodyStr, testVersion) {
		t.Errorf("layout missing version %q", testVersion)
	}
}

// TestLayoutHandler_SelectActiveTabViaQuery walks each known tab slug
// via ?tab=<slug>, verifies the correct placeholder body is rendered,
// and asserts the active-tab marker moves with the query.
func TestLayoutHandler_SelectActiveTabViaQuery(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.LayoutHandler))
	t.Cleanup(srv.Close)

	cases := []struct {
		slug        string
		wantBodyHas string
	}{
		{"live-tail", `id="live-tail-output"`},
		// sessions tab is now data-driven (Phase 2c S3); with the
		// nil-store skeleton path it renders the empty view shell.
		{"sessions", `class="tl-tab-panel tl-sessions"`},
		// crashes tab is data-driven (Phase 2c S4); nil-store path
		// renders the empty crashes panel.
		{"crashes", `class="tl-tab-panel tl-crashes"`},
		// S5 polish: the agents stub now renders the wider
		// "Phase 2d — coming soon" empty-state card. Marker check is
		// loose-coupled to the headline so a future copy edit doesn't
		// have to touch this test, only the dedicated agents test
		// (TestAgentsTabRendersComingSoon) below.
		{"agents", `class="tl-tab-panel tl-agents"`},
	}
	for _, c := range cases {
		t.Run(c.slug, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/dashboard?tab=" + c.slug)
			if err != nil {
				t.Fatalf("GET ?tab=%s: %v", c.slug, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status=%d, want 200", resp.StatusCode)
			}
			b, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(b), c.wantBodyHas) {
				t.Errorf("body missing %q\n--- body ---\n%s", c.wantBodyHas, b)
			}
		})
	}
}

// TestLayoutHandler_UnknownTabFallsBackToDefault confirms that an
// unknown ?tab=<slug> silently falls back to the default tab in the
// layout render. (TabHandler's posture is different — it 404s — and
// is tested separately below.)
func TestLayoutHandler_UnknownTabFallsBackToDefault(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.LayoutHandler))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/dashboard?tab=does-not-exist")
	if err != nil {
		t.Fatalf("GET unknown tab: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (silent fallback)", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `id="live-tail-output"`) {
		t.Errorf("fallback should render default live-tail body")
	}
}

// TestTabHandler_RendersBodyWithoutLayout — the htmx partial-swap path.
// Each tab's response is the bare placeholder body, no <html> envelope.
func TestTabHandler_RendersBodyWithoutLayout(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.TabHandler))
	t.Cleanup(srv.Close)

	cases := []struct {
		slug        string
		wantBodyHas string
	}{
		{"live-tail", `id="live-tail-output"`},
		// sessions tab is now data-driven (Phase 2c S3); the
		// nil-store skeleton path renders the empty view shell.
		{"sessions", `class="tl-tab-panel tl-sessions"`},
		// crashes tab is data-driven (Phase 2c S4); nil-store path
		// renders the empty crashes panel.
		{"crashes", `class="tl-tab-panel tl-crashes"`},
		// S5 polish: the agents stub now renders the wider
		// "Phase 2d — coming soon" empty-state card. Marker check is
		// loose-coupled to the headline so a future copy edit doesn't
		// have to touch this test, only the dedicated agents test
		// (TestAgentsTabRendersComingSoon) below.
		{"agents", `class="tl-tab-panel tl-agents"`},
	}
	for _, c := range cases {
		t.Run(c.slug, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/dashboard/tab/" + c.slug)
			if err != nil {
				t.Fatalf("GET tab %s: %v", c.slug, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status=%d, want 200", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			s := string(body)
			if !strings.Contains(s, c.wantBodyHas) {
				t.Errorf("body missing %q", c.wantBodyHas)
			}
			if strings.Contains(s, "<html") || strings.Contains(s, "<body") {
				t.Errorf("tab handler must not emit layout envelope; got %q", s)
			}
		})
	}
}

// TestTabHandler_UnknownTabReturns404 pins the partial-swap error
// posture: an htmx GET to a non-existent tab must 404 so a stale page
// link fails loud. Differs from LayoutHandler's silent fallback by
// design (see handler.go doc).
func TestTabHandler_UnknownTabReturns404(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.TabHandler))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/dashboard/tab/does-not-exist")
	if err != nil {
		t.Fatalf("GET unknown tab: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
}

// TestTabHandler_EmptySlug404s catches the "/dashboard/tab/" (no slug)
// case — must not silently render the default tab body.
func TestTabHandler_EmptySlug404s(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.TabHandler))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/dashboard/tab/")
	if err != nil {
		t.Fatalf("GET empty slug: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
}

// TestStaticHandler_ServesHTMXAndCSS verifies the embedded asset path:
// htmx.min.js and dashboard.css must be reachable from /dashboard/static/*
// with non-empty bodies. Sanity check that the //go:embed FS is wired
// through correctly.
func TestStaticHandler_ServesHTMXAndCSS(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.StaticHandler))
	t.Cleanup(srv.Close)

	cases := []struct {
		path        string
		wantContent string
	}{
		{"/dashboard/static/htmx.min.js", "htmx"},
		{"/dashboard/static/dashboard.css", "tl-header"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + c.path)
			if err != nil {
				t.Fatalf("GET %s: %v", c.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status=%d, want 200", resp.StatusCode)
			}
			b, _ := io.ReadAll(resp.Body)
			if len(b) == 0 {
				t.Errorf("%s: empty body", c.path)
			}
			if !strings.Contains(string(b), c.wantContent) {
				t.Errorf("%s: missing %q signature", c.path, c.wantContent)
			}
		})
	}
}

// TestStaticHandler_UnknownAsset404s verifies that unknown static paths
// return 404 (http.FileServer default).
func TestStaticHandler_UnknownAsset404s(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.StaticHandler))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/dashboard/static/does-not-exist.js")
	if err != nil {
		t.Fatalf("GET unknown asset: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", resp.StatusCode)
	}
}

// TestAgentsTabRendersComingSoon — S5 acceptance test for the
// agents-tab stub. Pins three contracts a future copy edit must keep:
//
//  1. The endpoint returns 200 (no 404 from a missing template, no
//     500 from a renderTabBody plumbing bug).
//  2. The body carries the Phase-2d bookmark — both "Phase 2d" and
//     "coming soon" must appear so the user sees the roadmap context
//     even if they only skim.
//  3. The tab-panel wrapper carries the consistent S5 marker class
//     ("tl-tab-panel tl-agents") so the empty-state styling stays in
//     sync with the sessions/crashes panels.
func TestAgentsTabRendersComingSoon(t *testing.T) {
	h := newHandler(t)
	srv := httptest.NewServer(http.HandlerFunc(h.TabHandler))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/dashboard/tab/agents")
	if err != nil {
		t.Fatalf("GET /dashboard/tab/agents: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "Phase 2d") {
		t.Errorf("agents body missing 'Phase 2d' marker; body:\n%s", s)
	}
	if !strings.Contains(s, "coming soon") {
		t.Errorf("agents body missing 'coming soon' marker; body:\n%s", s)
	}
	if !strings.Contains(s, `class="tl-tab-panel tl-agents"`) {
		t.Errorf("agents body missing 'tl-tab-panel tl-agents' wrapper class")
	}
	// Body-only contract: no <html> envelope on the htmx-swap path.
	if strings.Contains(s, "<html") || strings.Contains(s, "<body") {
		t.Errorf("tab handler must not emit layout envelope on agents tab")
	}
}
