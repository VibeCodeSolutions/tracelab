package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/VibeCodeSolutions/tracelab/internal/client"
)

// tailUpgrader mirrors the hub's upgrader (permissive CheckOrigin —
// bearer auth gates real-world access). For the client tests we add an
// explicit Authorization-header check before upgrading, which is what the
// production hub does at the chi-middleware layer.
var tailUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

// newTailServer returns an httptest.Server whose / handler implements a
// minimal /tail mock: bearer-auth check, then upgrade, then stream a
// caller-supplied sequence of events, then close cleanly.
//
// wantToken == "" disables the auth check (smoke mode).
//
// hook (when non-nil) is invoked AFTER the upgrade succeeds and BEFORE
// the events are streamed — tests use it to capture the query string the
// client sent (so we can assert ?session=… propagation).
func newTailServer(
	t *testing.T,
	wantToken string,
	events []client.Event,
	hook func(*http.Request, *websocket.Conn),
) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantToken != "" && r.Header.Get("Authorization") != "Bearer "+wantToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := tailUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		if hook != nil {
			hook(r, conn)
		}
		for _, e := range events {
			if err := conn.WriteJSON(e); err != nil {
				return
			}
		}
		// Send a clean close so the client returns nil from Tail.
		deadline := time.Now().Add(time.Second)
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			deadline,
		)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTail_HappyPath_DeliversEvents(t *testing.T) {
	t.Parallel()
	events := []client.Event{
		{SessionID: "s1", TS: 1700000000_000000000, Source: "logcat", Level: "INFO", Msg: "boot"},
		{SessionID: "s1", TS: 1700000001_000000000, Source: "logcat", Level: "WARN", Msg: "slow query"},
		{SessionID: "s1", TS: 1700000002_000000000, Source: "logcat", Level: "ERROR", Msg: "crash"},
	}
	srv := newTailServer(t, "tok", events, nil)

	c, err := client.New(client.Config{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	var got []client.Event
	var mu sync.Mutex
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = c.Tail(ctx, "", func(e client.Event) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(got) != len(events) {
		t.Fatalf("got %d events, want %d", len(got), len(events))
	}
	for i, e := range got {
		if e.Msg != events[i].Msg || e.Level != events[i].Level {
			t.Errorf("event[%d] = %+v, want %+v", i, e, events[i])
		}
	}
}

func TestTail_SessionFilter_PropagatesQuery(t *testing.T) {
	t.Parallel()
	var gotQuery string
	hook := func(r *http.Request, _ *websocket.Conn) {
		gotQuery = r.URL.RawQuery
	}
	srv := newTailServer(t, "tok", nil, hook)

	c, _ := client.New(client.Config{BaseURL: srv.URL, Token: "tok"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Tail(ctx, "sess-42", func(client.Event) {}); err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if gotQuery != "session=sess-42" {
		t.Errorf("query = %q, want session=sess-42", gotQuery)
	}
}

func TestTail_NoFilter_OmitsQuery(t *testing.T) {
	t.Parallel()
	var gotQuery string
	hook := func(r *http.Request, _ *websocket.Conn) {
		gotQuery = r.URL.RawQuery
	}
	srv := newTailServer(t, "tok", nil, hook)

	c, _ := client.New(client.Config{BaseURL: srv.URL, Token: "tok"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Tail(ctx, "", func(client.Event) {}); err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if gotQuery != "" {
		t.Errorf("expected no query, got %q", gotQuery)
	}
}

func TestTail_Unauthorized_ReturnsSentinel(t *testing.T) {
	t.Parallel()
	srv := newTailServer(t, "real", nil, nil) // server expects "real", we send "wrong"

	c, _ := client.New(client.Config{BaseURL: srv.URL, Token: "wrong"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.Tail(ctx, "", func(client.Event) { t.Fatal("onEvent must not fire on auth failure") })
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	if !errors.Is(err, client.ErrUnauthorized) {
		t.Errorf("err is not ErrUnauthorized: %v", err)
	}
	var he *client.HTTPError
	if !errors.As(err, &he) {
		t.Errorf("err is not *HTTPError: %v", err)
	} else if he.Status != http.StatusUnauthorized {
		t.Errorf("HTTPError.Status = %d, want 401", he.Status)
	}
}

func TestTail_ContextCancel_ReturnsNilCleanly(t *testing.T) {
	t.Parallel()
	// Server that upgrades, then blocks forever (no events, no close).
	// The client must exit cleanly when its context is cancelled.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := tailUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Wait for the client's close-frame (it should send one when its
		// ctx fires). ReadMessage returns an error then.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	c, _ := client.New(client.Config{BaseURL: srv.URL, Token: "tok"})
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after 100 ms — long enough for the handshake to complete.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		done <- c.Tail(ctx, "", func(client.Event) {})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Tail returned %v, want nil on ctx-cancel", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Tail did not return within 3 s of ctx-cancel — leaked goroutine?")
	}
}

func TestTail_RejectsNilCallback(t *testing.T) {
	t.Parallel()
	// No server needed — guard runs before any network IO.
	c, _ := client.New(client.Config{BaseURL: "http://127.0.0.1:1", Token: "tok"})
	err := c.Tail(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error for nil onEvent")
	}
	if !strings.Contains(err.Error(), "onEvent") {
		t.Errorf("error must name onEvent, got %v", err)
	}
}

// TestTail_MetaRoundTrip ensures Meta survives JSON encode/decode
// across the WS boundary — this is the only field with structural
// freedom and is the most likely drift point if Event ever sprouts a
// custom MarshalJSON.
func TestTail_MetaRoundTrip(t *testing.T) {
	t.Parallel()
	sent := []client.Event{{
		SessionID: "s1",
		Source:    "app",
		Level:     "INFO",
		Msg:       "with meta",
		Meta: map[string]any{
			"thread": "main",
			"count":  float64(7), // JSON numbers decode to float64
		},
	}}
	srv := newTailServer(t, "tok", sent, nil)
	c, _ := client.New(client.Config{BaseURL: srv.URL, Token: "tok"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var got client.Event
	err := c.Tail(ctx, "", func(e client.Event) { got = e })
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if got.Meta["thread"] != "main" || got.Meta["count"].(float64) != 7 {
		// Make the failure message helpful — JSON gives us very little
		// to go on if it silently drops a key.
		buf, _ := json.Marshal(got.Meta)
		t.Errorf("meta round-trip lost data: got %s", buf)
	}
}
