package http_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	httplayer "github.com/VibeCodeSolutions/tracelab/internal/http"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

const testToken = "test-token-deadbeef"

// newTestServer spins up a fresh SQLite-backed httptest.Server for each test.
// Cleanup of both the HTTP server and the store is registered via t.Cleanup.
func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
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
	return srv, st
}

func doJSON(t *testing.T, srv *httptest.Server, method, path, token string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func TestHealthzNoAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/healthz", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status field=%q, want ok", body["status"])
	}
}

func TestAuthRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	cases := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/session/start", map[string]string{"label": "x"}},
		{http.MethodPost, "/session/end", map[string]string{"session_id": "x"}},
		{http.MethodPost, "/ingest", map[string]any{"session_id": "x", "events": []any{}}},
		{http.MethodGet, "/sessions", nil},
		{http.MethodGet, "/events?session=x", nil},
		{http.MethodGet, "/crashes?session=x", nil},
	}
	for _, c := range cases {
		t.Run(c.method+c.path+"/no-token", func(t *testing.T) {
			resp := doJSON(t, srv, c.method, c.path, "", c.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("no-token status=%d, want 401", resp.StatusCode)
			}
		})
		t.Run(c.method+c.path+"/wrong-token", func(t *testing.T) {
			resp := doJSON(t, srv, c.method, c.path, "wrong", c.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("wrong-token status=%d, want 401", resp.StatusCode)
			}
		})
	}
}

func TestSessionStartEndFlow(t *testing.T) {
	srv, st := newTestServer(t)

	// Start session.
	resp := doJSON(t, srv, http.MethodPost, "/session/start", testToken, map[string]string{"label": "flow-test"})
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("start status=%d body=%s", resp.StatusCode, buf)
	}
	var startBody struct {
		SessionID string `json:"session_id"`
		StartedAt int64  `json:"started_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&startBody); err != nil {
		resp.Body.Close()
		t.Fatalf("decode start: %v", err)
	}
	resp.Body.Close()
	if startBody.SessionID == "" {
		t.Fatal("empty session_id")
	}

	// Ingest 3 events.
	events := []map[string]any{
		{"source": "app", "level": "INFO", "msg": "first"},
		{"source": "app", "level": "WARN", "msg": "second"},
		{"source": "app", "level": "ERROR", "msg": "third", "meta": map[string]string{"k": "v"}},
	}
	resp = doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
		"session_id": startBody.SessionID,
		"events":     events,
	})
	if resp.StatusCode != http.StatusAccepted {
		buf, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ingest status=%d body=%s", resp.StatusCode, buf)
	}
	var ingestBody struct {
		Ingested int `json:"ingested"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ingestBody); err != nil {
		resp.Body.Close()
		t.Fatalf("decode ingest: %v", err)
	}
	resp.Body.Close()
	if ingestBody.Ingested != 3 {
		t.Fatalf("ingested=%d, want 3", ingestBody.Ingested)
	}

	// End session.
	resp = doJSON(t, srv, http.MethodPost, "/session/end", testToken, map[string]string{"session_id": startBody.SessionID})
	if resp.StatusCode != http.StatusNoContent {
		buf, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("end status=%d body=%s", resp.StatusCode, buf)
	}
	resp.Body.Close()

	// Verify in store.
	got, err := st.RecentEvents(t.Context(), startBody.SessionID, 10)
	if err != nil {
		t.Fatalf("RecentEvents: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(events)=%d, want 3", len(got))
	}
	sessions, err := st.ListSessions(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != startBody.SessionID || sessions[0].EndedAt == nil {
		t.Fatalf("sessions=%+v", sessions)
	}
}

func TestIngestBatchInsert(t *testing.T) {
	srv, st := newTestServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/session/start", testToken, map[string]string{"label": "batch"})
	var startBody struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&startBody); err != nil {
		resp.Body.Close()
		t.Fatalf("decode start: %v", err)
	}
	resp.Body.Close()

	const N = 100
	events := make([]map[string]any, N)
	for i := 0; i < N; i++ {
		events[i] = map[string]any{"source": "bench", "level": "INFO", "msg": "evt"}
	}
	resp = doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
		"session_id": startBody.SessionID,
		"events":     events,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d, want 202", resp.StatusCode)
	}
	got, err := st.RecentEvents(t.Context(), startBody.SessionID, N+10)
	if err != nil {
		t.Fatalf("RecentEvents: %v", err)
	}
	if len(got) != N {
		t.Fatalf("len(events)=%d, want %d", len(got), N)
	}
}

func TestInvalidJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/session/start", strings.NewReader("not-json"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
}
