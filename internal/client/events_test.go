package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// TestEventsSince_HappyPath exercises a successful round-trip: the
// client passes session + since_seq + limit, the hub returns two
// events, and the decoded result carries SeqID, SessionID, Meta-as-map.
func TestEventsSince_HappyPath(t *testing.T) {
	var gotAuth, gotQuery string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/events" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"events":[
				{"seq_id":17,"session_id":"s1","ts":1700000000,"source":"a","level":"info","msg":"hello","meta":{"k":"v"}},
				{"seq_id":42,"session_id":"s1","ts":1700000001,"source":"a","level":"warn","msg":"world"}
			],
			"next_since_seq":42
		}`))
	})
	c, _ := newTestServer(t, h)

	events, next, err := c.EventsSince(context.Background(), "s1", 10, 2)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization=%q", gotAuth)
	}
	if gotQuery != "limit=2&session=s1&since_seq=10" {
		t.Errorf("query=%q, want canonical session/since_seq/limit", gotQuery)
	}
	if next != 42 {
		t.Errorf("next_since_seq=%d, want 42", next)
	}
	if len(events) != 2 {
		t.Fatalf("len=%d, want 2", len(events))
	}
	if events[0].SeqID != 17 || events[1].SeqID != 42 {
		t.Errorf("SeqIDs=[%d %d], want [17 42]", events[0].SeqID, events[1].SeqID)
	}
	if events[0].Meta == nil || events[0].Meta["k"] != "v" {
		t.Errorf("event[0].Meta=%v, want map[k:v]", events[0].Meta)
	}
	if events[1].Meta != nil {
		t.Errorf("event[1].Meta=%v, want nil (no meta in payload)", events[1].Meta)
	}
}

// TestEventsSince_EmptySession asserts the client-side fast-fail: an
// empty session id returns an error WITHOUT a network round-trip.
func TestEventsSince_EmptySession(t *testing.T) {
	var called bool
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	_, _, err := c.EventsSince(context.Background(), "", 0, 0)
	if err == nil {
		t.Fatal("expected error for empty session")
	}
	if called {
		t.Error("hub was contacted despite empty session — expected fail-fast")
	}
}

// TestEventsSince_OmitsZeroParams verifies that since_seq=0 and limit=0
// are omitted from the query string so the hub applies its defaults.
func TestEventsSince_OmitsZeroParams(t *testing.T) {
	var gotQuery string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"next_since_seq":0}`))
	})
	c, _ := newTestServer(t, h)
	_, _, err := c.EventsSince(context.Background(), "s1", 0, 0)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if gotQuery != "session=s1" {
		t.Errorf("query=%q, want only session=s1 (zeros omitted)", gotQuery)
	}
}

// TestEventsSince_StableNextOnEmpty asserts the "stable on empty"
// property propagates through the client: a hub response with
// events:[] + next_since_seq=<caller's-input> is returned verbatim.
func TestEventsSince_StableNextOnEmpty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"next_since_seq":100}`))
	})
	c, _ := newTestServer(t, h)
	events, next, err := c.EventsSince(context.Background(), "s1", 100, 0)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("len=%d, want 0", len(events))
	}
	if next != 100 {
		t.Errorf("next=%d, want 100 (stable on empty)", next)
	}
}

// TestEventsSince_Unauthorized maps a 401 to the ErrUnauthorized
// sentinel — same contract as every other authenticated client method.
func TestEventsSince_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	_, _, err := c.EventsSince(context.Background(), "s1", 0, 0)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err=%v, want ErrUnauthorized", err)
	}
}

// TestEventsSince_ServerError maps a 500 to ErrServerError — also the
// shared sentinel pattern.
func TestEventsSince_ServerError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `boom`, http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	_, _, err := c.EventsSince(context.Background(), "s1", 0, 0)
	if !errors.Is(err, ErrServerError) {
		t.Errorf("err=%v, want ErrServerError", err)
	}
}

// TestEventsSince_MalformedResponse covers the decoder failure path —
// a 200 with non-JSON body surfaces a decode error (not a sentinel).
func TestEventsSince_MalformedResponse(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	})
	c, _ := newTestServer(t, h)
	_, _, err := c.EventsSince(context.Background(), "s1", 0, 0)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

// TestEventsSince_BadRequest covers a 400 with a body — the response
// must surface as *HTTPError without sentinel wrapping (4xx other than
// 401/403 is not auth and not server).
func TestEventsSince_BadRequest(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"session required"}`, http.StatusBadRequest)
	})
	c, _ := newTestServer(t, h)
	_, _, err := c.EventsSince(context.Background(), "s1", 0, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.Status != http.StatusBadRequest {
		t.Errorf("status=%d", httpErr.Status)
	}
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrServerError) {
		t.Errorf("400 should not match Unauthorized/ServerError sentinels")
	}
}
