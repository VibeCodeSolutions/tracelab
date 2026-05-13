package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestIngest_OK(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/ingest" {
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
		}
		assertBearer(t, r, "test-token")
		body, _ := io.ReadAll(r.Body)
		var got ingestReqWire
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("bad body %s: %v", body, err)
		}
		if got.SessionID != "sess-1" {
			t.Errorf("session_id = %q", got.SessionID)
		}
		if len(got.Events) != 2 {
			t.Fatalf("events len = %d", len(got.Events))
		}
		// Event[0] omitted TS — must serialise without a ts field.
		if got.Events[0].TS != 0 {
			t.Errorf("events[0].ts should be 0, got %d", got.Events[0].TS)
		}
		// Event[1] carried meta — verify it survived round-trip.
		if got.Events[1].Source != "app" || got.Events[1].Level != "ERROR" {
			t.Errorf("events[1] = %+v", got.Events[1])
		}
		if len(got.Events[1].Meta) == 0 {
			t.Error("events[1].meta should not be empty")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ingested":2}`))
	})
	c, _ := newTestServer(t, h)
	n, err := c.Ingest(context.Background(), "sess-1", []Event{
		{Source: "app", Level: "INFO", Msg: "hello"},
		{Source: "app", Level: "ERROR", Msg: "boom", Meta: map[string]any{"k": "v"}},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 2 {
		t.Errorf("accepted = %d, want 2", n)
	}
}

func TestIngest_EmptyEvents(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var got ingestReqWire
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("bad body: %v", err)
		}
		if got.SessionID != "sess-x" {
			t.Errorf("session_id = %q", got.SessionID)
		}
		// An empty slice survives JSON as `"events":[]` — that's what the hub expects.
		if got.Events == nil || len(got.Events) != 0 {
			t.Errorf("events should be empty slice, got %v", got.Events)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ingested":0}`))
	})
	c, _ := newTestServer(t, h)
	n, err := c.Ingest(context.Background(), "sess-x", nil)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if n != 0 {
		t.Errorf("accepted = %d, want 0", n)
	}
}

func TestIngest_EmptySessionID(t *testing.T) {
	c, err := New(Config{BaseURL: "http://nope.invalid:1", Token: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Ingest(context.Background(), "", nil); err == nil {
		t.Fatal("expected error for empty sessionID")
	}
}

func TestIngest_Unauthorized(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
	c, _ := newTestServer(t, h)
	_, err := c.Ingest(context.Background(), "s", []Event{{Msg: "x"}})
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestIngest_ServerError(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	})
	c, _ := newTestServer(t, h)
	_, err := c.Ingest(context.Background(), "s", []Event{{Msg: "x"}})
	if !errors.Is(err, ErrServerError) {
		t.Errorf("expected ErrServerError, got %v", err)
	}
	// Body snippet should be captured.
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.Body == "" {
		t.Error("expected non-empty body snippet")
	}
}

func TestIngest_BadJSONResponse(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`not json`))
	})
	c, _ := newTestServer(t, h)
	_, err := c.Ingest(context.Background(), "s", []Event{{Msg: "x"}})
	if err == nil {
		t.Fatal("expected decode error on malformed 2xx response")
	}
}
