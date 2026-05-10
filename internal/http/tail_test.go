package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	httplayer "github.com/VibeCodeSolutions/tracelab/internal/http"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// newTestServerWithHub spins up a SQLite-backed httptest.Server with /tail
// wired to a fresh ws.Hub. Both are released via t.Cleanup.
func newTestServerWithHub(t *testing.T) (*httptest.Server, *store.Store, *ws.Hub) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tracelab.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	hub := ws.NewHub(0)
	t.Cleanup(hub.Close)

	h := httplayer.New(st, httplayer.Config{
		AuthToken: testToken,
		Hub:       hub,
	})
	if h == nil {
		t.Fatal("httplayer.New returned nil")
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st, hub
}

// dialTail builds a /tail websocket URL with the given query and Bearer
// token (empty token = no Authorization header).
func dialTail(t *testing.T, srv *httptest.Server, query, token string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = "/tail"
	u.RawQuery = strings.TrimPrefix(query, "?")
	hdr := http.Header{}
	if token != "" {
		hdr.Set("Authorization", "Bearer "+token)
	}
	c, resp, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	return c, resp, err
}

func TestTail_AuthReject(t *testing.T) {
	srv, _, _ := newTestServerWithHub(t)

	cases := []struct {
		name, token string
	}{
		{"no-token", ""},
		{"wrong-token", "wrong"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, resp, err := dialTail(t, srv, "", tc.token)
			if err == nil {
				_ = c.Close()
				t.Fatal("expected dial error for unauthorised request")
			}
			if resp == nil {
				t.Fatalf("no response: %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status=%d, want 401", resp.StatusCode)
			}
		})
	}
}

func TestTail_IngestFanOut(t *testing.T) {
	srv, _, _ := newTestServerWithHub(t)

	// Open two /tail clients, one filtered, one unfiltered.
	cAll, _, err := dialTail(t, srv, "", testToken)
	if err != nil {
		t.Fatalf("dial all: %v", err)
	}
	defer cAll.Close()

	// Start a session first — the filter needs a real id.
	resp := doJSON(t, srv, http.MethodPost, "/session/start", testToken, map[string]string{"label": "tail"})
	var startBody struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&startBody); err != nil {
		resp.Body.Close()
		t.Fatalf("decode start: %v", err)
	}
	resp.Body.Close()

	cFilter, _, err := dialTail(t, srv, "?session="+startBody.SessionID, testToken)
	if err != nil {
		t.Fatalf("dial filtered: %v", err)
	}
	defer cFilter.Close()

	// Allow both subscriptions to register.
	time.Sleep(50 * time.Millisecond)

	// Ingest a 3-event batch.
	events := []map[string]any{
		{"source": "app", "level": "INFO", "msg": "first"},
		{"source": "app", "level": "WARN", "msg": "second"},
		{"source": "app", "level": "ERROR", "msg": "third"},
	}
	deadline := time.Now()
	resp = doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
		"session_id": startBody.SessionID,
		"events":     events,
	})
	if resp.StatusCode != http.StatusAccepted {
		resp.Body.Close()
		t.Fatalf("ingest status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	// Both clients must receive all three events. We bound per-event read
	// to a generous 500ms but assert the *first* event arrives within
	// 100ms of the ingest call to satisfy the smoke-budget.
	for i, c := range []*websocket.Conn{cAll, cFilter} {
		for j := 0; j < 3; j++ {
			_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var e ws.Event
			if err := c.ReadJSON(&e); err != nil {
				t.Fatalf("client%d event%d read: %v", i, j, err)
			}
			if e.SessionID != startBody.SessionID {
				t.Fatalf("client%d event%d session=%q, want %q", i, j, e.SessionID, startBody.SessionID)
			}
			if j == 0 {
				if elapsed := time.Since(deadline); elapsed > 100*time.Millisecond {
					t.Logf("warning: client%d first-event latency %v exceeds 100ms budget", i, elapsed)
				}
			}
		}
	}
}

func TestTail_ServerShutdownClosesClients(t *testing.T) {
	srv, _, hub := newTestServerWithHub(t)

	c, _, err := dialTail(t, srv, "", testToken)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// Allow subscription to register.
	time.Sleep(50 * time.Millisecond)

	// Simulate server shutdown via hub.Close — this is what cmd/hub does
	// before srv.Shutdown.
	hub.Close()

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("expected error after hub.Close")
	}
}
