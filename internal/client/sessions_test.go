package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// assertBearer fails the test unless the request carries the expected
// Authorization: Bearer <token> header.
func assertBearer(t *testing.T, r *http.Request, token string) {
	t.Helper()
	got := r.Header.Get("Authorization")
	want := "Bearer " + token
	if got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestStartSession_OK(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/session/start" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		assertBearer(t, r, "test-token")
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		var got sessionStartReqWire
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("bad body %s: %v", body, err)
		}
		if got.Label != "smoke" {
			t.Errorf("label = %q", got.Label)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"sess-123","started_at":1700000000000000000}`))
	})
	c, _ := newTestServer(t, h)
	id, err := c.StartSession(context.Background(), "smoke")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if id != "sess-123" {
		t.Errorf("id = %q, want sess-123", id)
	}
}

func TestStartSession_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	_, err := c.StartSession(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestStartSession_ServerError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	_, err := c.StartSession(context.Background(), "x")
	if !errors.Is(err, ErrServerError) {
		t.Errorf("expected ErrServerError, got %v", err)
	}
}

func TestEndSession_OK_NoContent(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/session/end" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		assertBearer(t, r, "test-token")
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"session_id":"sess-9"`) {
			t.Errorf("body missing session_id: %s", body)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	c, _ := newTestServer(t, h)
	if err := c.EndSession(context.Background(), "sess-9"); err != nil {
		t.Errorf("EndSession: %v", err)
	}
}

func TestEndSession_EmptyID(t *testing.T) {
	// Should fail client-side without hitting the network.
	c, err := New(Config{BaseURL: "http://nope.invalid:1", Token: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.EndSession(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestEndSession_NotFound(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
	})
	c, _ := newTestServer(t, h)
	err := c.EndSession(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.Status != http.StatusNotFound {
		t.Errorf("Status = %d", httpErr.Status)
	}
	// 404 must NOT be sentinel-wrapped — only 401/403/5xx are.
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrServerError) {
		t.Errorf("404 should not match Unauthorized/ServerError sentinels")
	}
}

func TestEndSession_Forbidden(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	})
	c, _ := newTestServer(t, h)
	err := c.EndSession(context.Background(), "x")
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("403 must map to ErrUnauthorized, got %v", err)
	}
}

func TestListSessions_OK(t *testing.T) {
	endedAt := int64(1700000000123456789)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/sessions" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		if q := r.URL.Query().Get("limit"); q != "5" {
			t.Errorf("limit = %q, want 5", q)
		}
		assertBearer(t, r, "test-token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[
			{"id":"a","label":"first","started_at":1000,"ended_at":1700000000123456789},
			{"id":"b","label":"open","started_at":2000}
		]}`))
	})
	c, _ := newTestServer(t, h)
	sessions, err := c.ListSessions(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len = %d, want 2", len(sessions))
	}
	if sessions[0].ID != "a" || sessions[0].EndedAt == nil || *sessions[0].EndedAt != endedAt {
		t.Errorf("first session unexpected: %+v", sessions[0])
	}
	if sessions[1].EndedAt != nil {
		t.Errorf("second session EndedAt should be nil, got %v", *sessions[1].EndedAt)
	}
}

func TestListSessions_NoLimit_OmitsQuery(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query string, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[]}`))
	})
	c, _ := newTestServer(t, h)
	got, err := c.ListSessions(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", got)
	}
}

func TestListSessions_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	_, err := c.ListSessions(context.Background(), 10)
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestListSessions_ServerError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `oops`, http.StatusBadGateway)
	})
	c, _ := newTestServer(t, h)
	_, err := c.ListSessions(context.Background(), 10)
	if !errors.Is(err, ErrServerError) {
		t.Errorf("expected ErrServerError for 502, got %v", err)
	}
}
