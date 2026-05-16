package dashboard_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/dashboard"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// newStreamServer mounts h.StreamHandler(hub) at /dashboard/stream on a
// httptest.Server. The handler is the production handler under test.
func newStreamServer(t *testing.T, hub *ws.Hub) (*httptest.Server, *dashboard.Handler) {
	t.Helper()
	h, err := dashboard.NewHandler("test", nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard/stream", h.StreamHandler(hub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, h
}

// openStream issues GET /dashboard/stream with the given session
// query value and returns the live response. Caller must close it.
func openStream(t *testing.T, srv *httptest.Server, session string) *http.Response {
	t.Helper()
	url := srv.URL + "/dashboard/stream"
	if session != "" {
		url += "?session=" + session
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

// readFrame reads one SSE event or comment frame (terminated by "\n\n").
// Returns the raw frame bytes (without the trailing blank line) or an
// error/EOF.
func readFrame(r *bufio.Reader) (string, error) {
	var sb strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if sb.Len() > 0 {
				return sb.String(), nil
			}
			return "", err
		}
		if line == "\n" || line == "\r\n" {
			return sb.String(), nil
		}
		sb.WriteString(line)
	}
}

// TestSSEContentType pins the SSE response headers. The handler must
// set text/event-stream + no-cache before flushing the status line so
// proxies don't buffer the stream.
func TestSSEContentType(t *testing.T) {
	hub := ws.NewHub(8)
	defer hub.Close()
	srv, _ := newStreamServer(t, hub)

	resp := openStream(t, srv, "sess-1")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type=%q, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control=%q, want no-cache", cc)
	}
	if conn := resp.Header.Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection=%q, want keep-alive", conn)
	}
	if xa := resp.Header.Get("X-Accel-Buffering"); xa != "no" {
		t.Errorf("X-Accel-Buffering=%q, want no", xa)
	}
}

// TestSSEDataFormat publishes one event and asserts the on-wire frame
// is "data: <json>\n\n" with the JSON payload round-tripping through
// ws.Event verbatim.
func TestSSEDataFormat(t *testing.T) {
	hub := ws.NewHub(8)
	defer hub.Close()
	srv, _ := newStreamServer(t, hub)

	resp := openStream(t, srv, "sess-1")
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)

	// Give the handler a moment to subscribe before publishing.
	waitForSubscribers(t, hub, 1)

	evt := ws.Event{
		SessionID: "sess-1",
		TS:        1700000000_000_000_000,
		Source:    "test",
		Level:     "INFO",
		Msg:       "hello sse",
	}
	hub.Publish(evt)

	frame, err := readFrameWithTimeout(reader, 2*time.Second)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if !strings.HasPrefix(frame, "data: ") {
		t.Fatalf("frame=%q, want leading 'data: '", frame)
	}
	jsonPart := strings.TrimSuffix(strings.TrimPrefix(frame, "data: "), "\n")
	var got ws.Event
	if err := json.Unmarshal([]byte(jsonPart), &got); err != nil {
		t.Fatalf("unmarshal payload: %v (raw=%q)", err, jsonPart)
	}
	if got.SessionID != evt.SessionID || got.Msg != evt.Msg ||
		got.Source != evt.Source || got.Level != evt.Level || got.TS != evt.TS {
		t.Errorf("event mismatch: got=%+v want=%+v", got, evt)
	}
}

// TestSSEHeartbeat shrinks HeartbeatInterval and observes that the
// handler emits a ": heartbeat" SSE comment frame even when no event
// is published. Comment lines are how SSE keeps the connection alive
// across idle proxies.
func TestSSEHeartbeat(t *testing.T) {
	prev := dashboard.HeartbeatInterval
	dashboard.HeartbeatInterval = 50 * time.Millisecond
	defer func() { dashboard.HeartbeatInterval = prev }()

	hub := ws.NewHub(8)
	defer hub.Close()
	srv, _ := newStreamServer(t, hub)

	resp := openStream(t, srv, "sess-1")
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)

	frame, err := readFrameWithTimeout(reader, 1*time.Second)
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	if !strings.HasPrefix(frame, ":") {
		t.Errorf("expected SSE comment frame (starts with ':'), got %q", frame)
	}
	if !strings.Contains(frame, "heartbeat") {
		t.Errorf("heartbeat payload missing 'heartbeat' marker: %q", frame)
	}
}

// TestSSESlowSubscriberDrop asserts that when the Hub's per-subscriber
// channel is full, ws.Hub.Publish drops events for that subscriber
// (the existing Hub contract — see ADR-012 Consequences). The SSE
// handler inherits this behaviour: the server stays healthy, the
// stream stays open, and only the dropped events are missing.
//
// The test publishes more events than the Hub buffer can hold without
// the consumer reading from the response body. The handler's internal
// flush will eventually block on the net.Conn write buffer, but the
// publisher must never block on the Hub channel (Hub.Publish uses
// non-blocking sends). We verify the publisher path completes
// promptly even with N events on a buffer of size 2.
func TestSSESlowSubscriberDrop(t *testing.T) {
	hub := ws.NewHub(2) // tiny buffer to force drops fast
	defer hub.Close()
	srv, _ := newStreamServer(t, hub)

	resp := openStream(t, srv, "sess-1")
	defer resp.Body.Close()

	// Do NOT read from resp.Body — that's the "slow subscriber".
	waitForSubscribers(t, hub, 1)

	// Publish many events; Hub.Publish is non-blocking by contract.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			hub.Publish(ws.Event{SessionID: "sess-1", Msg: "burst"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publisher blocked — slow-subscriber-drop contract violated")
	}
}

// TestSSEContextCancelCleanup verifies that disconnecting the client
// (closing the http.Response) causes the handler to call
// Hub.unsubscribe, freeing the slot. SubscriberCount drops back to
// zero within a short window.
func TestSSEContextCancelCleanup(t *testing.T) {
	hub := ws.NewHub(8)
	defer hub.Close()
	srv, _ := newStreamServer(t, hub)

	beforeGoroutines := runtime.NumGoroutine()

	// Open and close in a tight loop to stress the cleanup path.
	for i := 0; i < 5; i++ {
		resp := openStream(t, srv, "sess-1")
		waitForSubscribers(t, hub, 1)
		resp.Body.Close()
		waitForSubscribers(t, hub, 0)
	}

	if got := hub.SubscriberCount(); got != 0 {
		t.Errorf("SubscriberCount after cleanup=%d, want 0", got)
	}
	// Give the runtime a moment to retire the handler goroutines.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > beforeGoroutines+2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if delta := runtime.NumGoroutine() - beforeGoroutines; delta > 2 {
		// Allow small drift from httptest server goroutines.
		t.Errorf("goroutine leak suspected: delta=%d (before=%d, after=%d)",
			delta, beforeGoroutines, runtime.NumGoroutine())
	}
}

// TestSSEUnknownSession — unknown session id is NOT a 404. The
// dashboard /stream endpoint accepts any non-empty session value and
// returns an empty stream (no events match the filter). This mirrors
// the /events and /tail posture: session existence is a /sessions
// concern, not a stream-endpoint concern. The handler stays open
// until the client disconnects or a heartbeat fires.
func TestSSEUnknownSession(t *testing.T) {
	prev := dashboard.HeartbeatInterval
	dashboard.HeartbeatInterval = 100 * time.Millisecond
	defer func() { dashboard.HeartbeatInterval = prev }()

	hub := ws.NewHub(8)
	defer hub.Close()
	srv, _ := newStreamServer(t, hub)

	resp := openStream(t, srv, "definitely-not-a-real-session")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (unknown session = empty stream)", resp.StatusCode)
	}
	// Reading should yield only heartbeats (no events ever match).
	reader := bufio.NewReader(resp.Body)
	frame, err := readFrameWithTimeout(reader, 1*time.Second)
	if err != nil {
		t.Fatalf("read frame from empty stream: %v", err)
	}
	if !strings.HasPrefix(frame, ":") {
		t.Errorf("unknown session should yield only heartbeat frames, got %q", frame)
	}
}

// TestSSESessionRequired — missing/empty session query param is 400.
// The S2 design picks "session required" rather than "stream all
// sessions" because the dashboard always knows which session the
// user picked (the /stream call is the consequence of a UI action).
// A no-session call is almost certainly a client bug; failing loud
// is better than silently fanning out the global event firehose.
func TestSSESessionRequired(t *testing.T) {
	hub := ws.NewHub(8)
	defer hub.Close()
	srv, _ := newStreamServer(t, hub)

	resp := openStream(t, srv, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "session required") {
		t.Errorf("body=%q, expected 'session required'", body)
	}
}

// TestSSEHubNil — when cfg.Hub is nil in internal/http, the dashboard
// /stream route is not registered. That's exercised in the http-layer
// wireup tests; here we cover the handler-construction path: passing
// a nil hub to StreamHandler must not panic, but the route shouldn't
// be reachable in production (the http layer guards it). Since the
// dashboard package doesn't enforce the guard itself, this test pins
// the documented posture by exercising the http-layer guard via a
// thin scenario: registering the handler only when hub != nil.
//
// We assert StreamHandler(nil) returns a non-nil HandlerFunc (the
// production layer skips registration entirely; this test is the
// inverse-direction sanity check). The handler under hub=nil would
// panic on Subscribe — that's deliberate, the http layer must not
// register it. We verify the http layer's guard separately in
// internal/http/dashboard_stream_wireup_test.go.
func TestSSEHubNil(t *testing.T) {
	h, err := dashboard.NewHandler("test", nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	// StreamHandler is a factory; passing nil must still return a
	// HandlerFunc value (Go-idiomatic — nil-guard happens at the
	// http.Handler-registration site, not inside the factory).
	if h.StreamHandler(nil) == nil {
		t.Fatal("StreamHandler returned nil HandlerFunc")
	}
}

// ----- helpers -----------------------------------------------------

// waitForSubscribers spins until hub.SubscriberCount == want or 1s.
// The SSE handler calls Subscribe asynchronously after WriteHeader,
// so tests that publish into the Hub must wait for the subscription
// to register before publishing — otherwise the event is fanned out
// to zero subscribers and the test deadlocks on the read.
func waitForSubscribers(t *testing.T, hub *ws.Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waitForSubscribers: count=%d, want %d", hub.SubscriberCount(), want)
}

// readFrameWithTimeout wraps readFrame with a wall-clock cap. The
// underlying bufio.Reader blocks indefinitely on a slow stream, so
// tests use this to fail loud instead of hanging.
func readFrameWithTimeout(r *bufio.Reader, d time.Duration) (string, error) {
	type result struct {
		frame string
		err   error
	}
	ch := make(chan result, 1)
	var once sync.Once
	go func() {
		f, e := readFrame(r)
		once.Do(func() { ch <- result{f, e} })
	}()
	select {
	case res := <-ch:
		return res.frame, res.err
	case <-time.After(d):
		return "", errors.New("readFrameWithTimeout: timeout")
	}
}
