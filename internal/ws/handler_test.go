package ws_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// dialTail upgrades a fresh ws connection against srv and returns the conn.
// query is appended verbatim to the path (with leading `?` if non-empty).
func dialTail(t *testing.T, srv *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = "/tail"
	u.RawQuery = strings.TrimPrefix(query, "?")
	c, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial /tail: %v (status=%d)", err, statusOf(resp))
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func statusOf(resp *http.Response) int {
	if resp == nil {
		return -1
	}
	return resp.StatusCode
}

// readEvent reads one event JSON frame with a deadline.
func readEvent(t *testing.T, c *websocket.Conn, timeout time.Duration) (ws.Event, error) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	var e ws.Event
	mt, data, err := c.ReadMessage()
	if err != nil {
		return e, err
	}
	if mt != websocket.TextMessage {
		t.Fatalf("unexpected message type %d", mt)
	}
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unmarshal event: %v (raw=%s)", err, data)
	}
	return e, nil
}

func TestHandler_SubscribeAndReceive(t *testing.T) {
	hub := ws.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(ws.Handler(hub, nil))
	defer srv.Close()

	c := dialTail(t, srv, "")
	// Give the server a moment to register the subscriber.
	waitForSubs(t, hub, 1, time.Second)

	hub.Publish(ws.Event{SessionID: "s1", Msg: "hello", Source: "app", Level: "INFO", TS: 42})
	got, err := readEvent(t, c, time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.SessionID != "s1" || got.Msg != "hello" {
		t.Fatalf("unexpected event: %+v", got)
	}
}

func TestHandler_SessionFilter(t *testing.T) {
	hub := ws.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(ws.Handler(hub, nil))
	defer srv.Close()

	c := dialTail(t, srv, "?session=s1")
	waitForSubs(t, hub, 1, time.Second)

	hub.Publish(ws.Event{SessionID: "s2", Msg: "skip"})
	hub.Publish(ws.Event{SessionID: "s1", Msg: "take"})

	got, err := readEvent(t, c, time.Second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.SessionID != "s1" || got.Msg != "take" {
		t.Fatalf("filter leaked: got %+v", got)
	}
}

func TestHandler_DisconnectRemovesSubscriber(t *testing.T) {
	hub := ws.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(ws.Handler(hub, nil))
	defer srv.Close()

	c := dialTail(t, srv, "")
	waitForSubs(t, hub, 1, time.Second)

	// Client closes cleanly.
	_ = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
	_ = c.Close()

	waitForSubs(t, hub, 0, 2*time.Second)
}

func TestHandler_HeartbeatTimeout(t *testing.T) {
	// Make the server heartbeat aggressive so the test runs in <1s.
	origPing, origPong := ws.PingPeriod, ws.PongWait
	ws.PingPeriod = 50 * time.Millisecond
	ws.PongWait = 150 * time.Millisecond
	t.Cleanup(func() {
		ws.PingPeriod = origPing
		ws.PongWait = origPong
	})

	hub := ws.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(ws.Handler(hub, nil))
	defer srv.Close()

	// Dial as a "dumb" client that never replies to pings.
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = "/tail"
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	// Replace the default pong-reply behaviour: explicitly drop pings on the
	// floor by setting a no-op ping handler.
	c.SetPingHandler(func(string) error { return nil })

	waitForSubs(t, hub, 1, time.Second)

	// Drive the read pump on the client side so the conn observes the
	// ping frames, but never reply with a pong. The server should close
	// the connection after PongWait.
	gotClose := make(chan struct{})
	go func() {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				close(gotClose)
				return
			}
		}
	}()

	select {
	case <-gotClose:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not close dead client within 2s")
	}
	waitForSubs(t, hub, 0, 2*time.Second)
}

func TestHandler_HubCloseDisconnectsClients(t *testing.T) {
	hub := ws.NewHub(8)
	srv := httptest.NewServer(ws.Handler(hub, nil))
	defer srv.Close()

	c := dialTail(t, srv, "")
	waitForSubs(t, hub, 1, time.Second)

	// Close the hub: server must send a close frame and unsubscribe.
	hub.Close()

	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := c.ReadMessage()
	if err == nil {
		t.Fatal("expected close error after hub.Close")
	}
	// Either CloseGoingAway (if frame arrived first) or another close error
	// is acceptable — we just need the read to terminate.
	if ce, ok := err.(*websocket.CloseError); ok {
		if ce.Code != websocket.CloseGoingAway && ce.Code != websocket.CloseAbnormalClosure {
			t.Fatalf("unexpected close code %d", ce.Code)
		}
	}
}

// TestHandler_TwoClientsParallel verifies that both subscribers receive a
// publish, satisfying the "fan-out" smoke requirement at the hub level.
func TestHandler_TwoClientsParallel(t *testing.T) {
	hub := ws.NewHub(8)
	defer hub.Close()
	srv := httptest.NewServer(ws.Handler(hub, nil))
	defer srv.Close()

	c1 := dialTail(t, srv, "")
	c2 := dialTail(t, srv, "")
	waitForSubs(t, hub, 2, time.Second)

	var got1, got2 atomic.Int32
	wait := make(chan struct{}, 2)
	go func() {
		if e, err := readEvent(t, c1, time.Second); err == nil && e.Msg == "shared" {
			got1.Add(1)
		}
		wait <- struct{}{}
	}()
	go func() {
		if e, err := readEvent(t, c2, time.Second); err == nil && e.Msg == "shared" {
			got2.Add(1)
		}
		wait <- struct{}{}
	}()

	hub.Publish(ws.Event{SessionID: "s1", Msg: "shared"})
	<-wait
	<-wait

	if got1.Load() != 1 || got2.Load() != 1 {
		t.Fatalf("fan-out failed: c1=%d c2=%d", got1.Load(), got2.Load())
	}
}

// waitForSubs polls hub.SubscriberCount() until it matches want or the
// timeout expires. Used to deflake tests that depend on the read pump
// having registered the subscriber.
func waitForSubs(t *testing.T, h *ws.Hub, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.SubscriberCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("subscriber count = %d, want %d (timeout)", h.SubscriberCount(), want)
}
