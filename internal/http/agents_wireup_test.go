package http

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/VibeCodeSolutions/tracelab/internal/agents"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// newAgentsTestServer wires up a real httptest.Server with the
// bearer-auth + agents handler enabled. Mirrors the dashboard_wireup
// helpers so the wire-up tests stay parallel in shape across phases.
func newAgentsTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "agents-wireup.db")
	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	handler := New(st, Config{
		AuthToken: "secret-token",
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Agents:    agents.NewHandler(st, slog.Default()),
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, "secret-token"
}

// TestAgentsIngestRequiresBearer pins that /agents/ingest sits inside
// the bearer-protected sub-group — a request without an
// Authorization header is rejected with 401 BEFORE the handler ever
// sees the body. This is the same 401-posture as /ingest, /events,
// /crashes, /sessions, and the /adb/* endpoints.
func TestAgentsIngestRequiresBearer(t *testing.T) {
	srv, _ := newAgentsTestServer(t)

	body := bytes.NewReader([]byte(`{"source":"sdk-hook"}`))
	resp, err := http.Post(srv.URL+"/agents/ingest", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", resp.StatusCode)
	}
}

// TestAgentsIngestAcceptsValidBearer pins the happy path through the
// bearer middleware. The actual ingest semantics are covered in
// internal/agents/handler_test — here we only want to know the
// bearer + route wireup is correct.
func TestAgentsIngestAcceptsValidBearer(t *testing.T) {
	srv, token := newAgentsTestServer(t)

	body := bytes.NewReader([]byte(`{
		"source":"sdk-hook",
		"spawn":{
			"id":"01234567890123456789abcdef",
			"skill":"ballard",
			"started_at":1700000000000000000,
			"project":"tracelab"
		}
	}`))
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/agents/ingest", body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("status: want 200, got %d (body=%s)", resp.StatusCode, string(respBody))
	}
}

// TestAgentsIngestOmittedWhenNil is the regression-guard for the
// `cfg.Agents != nil` conditional in server.go — a hub built without
// the agents handler does NOT expose /agents/ingest at all (404
// inside the bearer group, NOT 405 or 401). Mirrors the
// dashboard_wireup `OmittedWhenNil` test pattern.
func TestAgentsIngestOmittedWhenNil(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "no-agents.db")
	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	handler := New(st, Config{
		AuthToken: "secret-token",
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		// Agents intentionally nil.
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	body := bytes.NewReader([]byte(`{"source":"sdk-hook"}`))
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/agents/ingest", body)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status: want 404 (route not registered), got %d", resp.StatusCode)
	}
}
