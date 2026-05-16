package http_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VibeCodeSolutions/tracelab/internal/dashboard"
	httplayer "github.com/VibeCodeSolutions/tracelab/internal/http"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// newDashboardServer spins up a fresh httptest.Server with both the
// bearer-guarded API and the Phase-2c dashboard sub-router wired up.
// Mirrors newTestServer but threads a non-nil dashboard.Handler through
// httplayer.Config.
func newDashboardServer(t *testing.T) *httptest.Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tracelab.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	dash, err := dashboard.NewHandler("test", nil, st)
	if err != nil {
		t.Fatalf("dashboard.NewHandler: %v", err)
	}
	h := httplayer.New(st, httplayer.Config{
		AuthToken: testToken,
		Dashboard: dash,
	})
	if h == nil {
		t.Fatal("httplayer.New returned nil")
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// TestDashboard_LayoutRoute pins the /dashboard route inside the
// combined router: GET /dashboard returns the layout HTML, no auth
// required (S1 auth posture — see Config.Dashboard doc).
func TestDashboard_LayoutRoute(t *testing.T) {
	srv := newDashboardServer(t)
	resp, err := http.Get(srv.URL + "/dashboard")
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Tracelab") {
		t.Errorf("layout missing 'Tracelab' brand")
	}
	if !strings.Contains(string(body), "Live-Tail") {
		t.Errorf("layout missing live-tail tab label")
	}
}

// TestDashboard_TabRoute pins the htmx-swap partial route through the
// combined router.
func TestDashboard_TabRoute(t *testing.T) {
	srv := newDashboardServer(t)
	resp, err := http.Get(srv.URL + "/dashboard/tab/sessions")
	if err != nil {
		t.Fatalf("GET /dashboard/tab/sessions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// Phase 2c S3 — sessions tab is now data-driven; we assert on the
	// stable structural marker (panel class) rather than a placeholder
	// string. The combined router test only verifies the route lands
	// in the dashboard handler; the rich content + sort/filter/page
	// behaviour is covered in internal/dashboard/sessions_test.go.
	if !strings.Contains(string(body), `class="tl-tab-panel tl-sessions"`) {
		t.Errorf("tab response missing sessions tab marker")
	}
}

// TestDashboard_StaticRoute pins the embedded asset route through the
// combined router. Asserts htmx.min.js is reachable and non-empty.
func TestDashboard_StaticRoute(t *testing.T) {
	srv := newDashboardServer(t)
	resp, err := http.Get(srv.URL + "/dashboard/static/htmx.min.js")
	if err != nil {
		t.Fatalf("GET htmx: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) < 1000 {
		t.Errorf("htmx asset suspiciously small: %d bytes", len(body))
	}
}

// TestDashboard_OmittedWhenNil — Config.Dashboard=nil means the routes
// are not registered, so GET /dashboard returns 404. Regression guard
// that ensures HTTP-layer unit tests that don't supply a Dashboard
// keep the same shape they had before P2c-S1.
func TestDashboard_OmittedWhenNil(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tracelab.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	h := httplayer.New(st, httplayer.Config{AuthToken: testToken})
	if h == nil {
		t.Fatal("httplayer.New returned nil")
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/dashboard")
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 (dashboard omitted)", resp.StatusCode)
	}
}
