package client

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

// TestCrashesList_HappyPath exercises a successful round-trip: the
// client passes session_id + limit, the hub returns two crashes, and the
// decoded result carries ID, SessionID, ts, fingerprint, count.
func TestCrashesList_HappyPath(t *testing.T) {
	var gotAuth, gotQuery, gotPath string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"crashes":[
				{"id":42,"session_id":"s1","ts":1700000002,"fingerprint":"fp-2","stacktrace":"trace 2","count":3},
				{"id":17,"session_id":"s1","ts":1700000001,"fingerprint":"fp-1","stacktrace":"trace 1","count":1}
			]
		}`))
	})
	c, _ := newTestServer(t, h)

	crashes, err := c.CrashesList(context.Background(), "s1", 10)
	if err != nil {
		t.Fatalf("CrashesList: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization=%q", gotAuth)
	}
	if gotPath != "/crashes" {
		t.Errorf("path=%q, want /crashes", gotPath)
	}
	if gotQuery != "limit=10&session=s1" {
		t.Errorf("query=%q, want canonical limit=10&session=s1", gotQuery)
	}
	if len(crashes) != 2 {
		t.Fatalf("len=%d, want 2", len(crashes))
	}
	if crashes[0].ID != 42 || crashes[1].ID != 17 {
		t.Errorf("IDs=[%d %d], want [42 17] (newest first)", crashes[0].ID, crashes[1].ID)
	}
	if crashes[0].Fingerprint != "fp-2" || crashes[0].Count != 3 {
		t.Errorf("crashes[0]=%+v, want fp-2/count=3", crashes[0])
	}
}

// TestCrashesList_EmptySession asserts the client-side fast-fail: an
// empty session id returns an error WITHOUT a network round-trip.
func TestCrashesList_EmptySession(t *testing.T) {
	var called bool
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	_, err := c.CrashesList(context.Background(), "", 0)
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
	if called {
		t.Error("hub was contacted despite empty session — expected fail-fast")
	}
}

// TestCrashesList_OmitsZeroLimit verifies that limit=0 is omitted from
// the query string so the hub applies its default. Mirror of
// TestEventsSince_OmitsZeroParams.
func TestCrashesList_OmitsZeroLimit(t *testing.T) {
	var gotQuery string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crashes":[]}`))
	})
	c, _ := newTestServer(t, h)
	_, err := c.CrashesList(context.Background(), "s1", 0)
	if err != nil {
		t.Fatalf("CrashesList: %v", err)
	}
	if gotQuery != "session=s1" {
		t.Errorf("query=%q, want only session=s1 (zero limit omitted)", gotQuery)
	}
}

// TestCrashesList_EmptyResultNotNil asserts that a hub response with
// crashes:[] returns an empty slice, not nil. The MCP tool layer relies
// on this so JSON output emits "[]" rather than "null".
func TestCrashesList_EmptyResultNotNil(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crashes":[]}`))
	})
	c, _ := newTestServer(t, h)
	crashes, err := c.CrashesList(context.Background(), "s1", 0)
	if err != nil {
		t.Fatalf("CrashesList: %v", err)
	}
	if crashes == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(crashes) != 0 {
		t.Errorf("len=%d, want 0", len(crashes))
	}
}

// TestCrashesList_Unauthorized maps a 401 to the ErrUnauthorized
// sentinel — same contract as every other authenticated client method.
func TestCrashesList_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	_, err := c.CrashesList(context.Background(), "s1", 0)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err=%v, want ErrUnauthorized", err)
	}
}

// TestCrashesList_ServerError maps a 500 to ErrServerError — also the
// shared sentinel pattern.
func TestCrashesList_ServerError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `boom`, http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	_, err := c.CrashesList(context.Background(), "s1", 0)
	if !errors.Is(err, ErrServerError) {
		t.Errorf("err=%v, want ErrServerError", err)
	}
}

// TestCrashesList_MalformedResponse covers the decoder failure path —
// a 200 with non-JSON body surfaces a decode error (not a sentinel).
func TestCrashesList_MalformedResponse(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	})
	c, _ := newTestServer(t, h)
	_, err := c.CrashesList(context.Background(), "s1", 0)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
}

// TestCrashesList_BadRequest covers a 400 with a body — the response
// must surface as *HTTPError without sentinel wrapping.
func TestCrashesList_BadRequest(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"session required"}`, http.StatusBadRequest)
	})
	c, _ := newTestServer(t, h)
	_, err := c.CrashesList(context.Background(), "s1", 0)
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
