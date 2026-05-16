package http_test

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/dashboard"
	httplayer "github.com/VibeCodeSolutions/tracelab/internal/http"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// newDashboardStreamServer wires the combined router with both a
// Dashboard handler and a Hub, so /dashboard/stream is registered
// (the Hub-nil branch in server.go skips the route).
func newDashboardStreamServer(t *testing.T) (*httptest.Server, *ws.Hub) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tracelab.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	dash, err := dashboard.NewHandler("test", nil, nil)
	if err != nil {
		t.Fatalf("dashboard.NewHandler: %v", err)
	}
	hub := ws.NewHub(8)
	t.Cleanup(hub.Close)

	h := httplayer.New(st, httplayer.Config{
		AuthToken: testToken,
		Hub:       hub,
		Dashboard: dash,
	})
	if h == nil {
		t.Fatal("httplayer.New returned nil")
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, hub
}

// TestDashboardStream_RegisteredWhenHubAndDashboardPresent — the
// /dashboard/stream route exists when both cfg.Hub and cfg.Dashboard
// are non-nil. GET returns 200 + text/event-stream.
func TestDashboardStream_RegisteredWhenHubAndDashboardPresent(t *testing.T) {
	srv, _ := newDashboardStreamServer(t)
	resp, err := http.Get(srv.URL + "/dashboard/stream?session=sess-1")
	if err != nil {
		t.Fatalf("GET /dashboard/stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type=%q, want text/event-stream", ct)
	}
}

// TestDashboardStream_NotRegisteredWhenHubNil — when cfg.Hub is nil
// the route is omitted (mirroring the /tail-WS posture). The chi
// router returns 404, NOT a 500 from a panicking nil-Hub Subscribe.
func TestDashboardStream_NotRegisteredWhenHubNil(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tracelab.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	dash, err := dashboard.NewHandler("test", nil, nil)
	if err != nil {
		t.Fatalf("dashboard.NewHandler: %v", err)
	}
	h := httplayer.New(st, httplayer.Config{
		AuthToken: testToken,
		Dashboard: dash,
		// Hub deliberately nil.
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/dashboard/stream?session=sess-1")
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 (stream omitted when Hub=nil)", resp.StatusCode)
	}
}

// TestDashboardStream_NoBearerRequired — the dashboard sub-router is
// permanently Loopback-only (ADR-011 Consequences). /dashboard/stream
// must NOT require a bearer; sending one is also harmless (just
// ignored). This pins the auth posture so a future careless edit
// can't silently re-introduce the bearer requirement.
func TestDashboardStream_NoBearerRequired(t *testing.T) {
	srv, _ := newDashboardStreamServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/dashboard/stream?session=sess-1", nil)
	// No Authorization header.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (no bearer required)", resp.StatusCode)
	}
}

// TestDashboardStream_EventEndToEnd publishes via the Hub and asserts
// the SSE consumer receives the data frame end-to-end through the
// combined router (request-id, recoverer, slog middlewares applied).
// Regression guard that none of the middlewares strip the streaming
// flush.
func TestDashboardStream_EventEndToEnd(t *testing.T) {
	srv, hub := newDashboardStreamServer(t)

	resp, err := http.Get(srv.URL + "/dashboard/stream?session=sess-e2e")
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)

	// Wait for Subscribe to register.
	deadline := time.Now().Add(1 * time.Second)
	for hub.SubscriberCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.SubscriberCount() == 0 {
		t.Fatal("stream handler did not subscribe")
	}

	hub.Publish(ws.Event{SessionID: "sess-e2e", Msg: "wired-through"})

	// Read until we see our marker — heartbeats may interleave.
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		var acc strings.Builder
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				acc.Write(buf[:n])
				if strings.Contains(acc.String(), "wired-through") {
					done <- acc.String()
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					done <- acc.String()
				}
				return
			}
		}
	}()
	select {
	case got := <-done:
		if !strings.Contains(got, "wired-through") {
			t.Errorf("E2E body missing event payload: %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("E2E timeout — event not surfaced through combined router")
	}
}
