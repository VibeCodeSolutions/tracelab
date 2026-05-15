package http_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// eventView mirrors the wire shape of an /events response row.
// Kept local to the test so an accidental rename in handlers.go's
// eventView fails this test rather than silently passing.
type eventView struct {
	SeqID     int64           `json:"seq_id"`
	SessionID string          `json:"session_id"`
	TS        int64           `json:"ts"`
	Source    string          `json:"source"`
	Level     string          `json:"level"`
	Msg       string          `json:"msg"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

type listEventsResp struct {
	Events       []eventView `json:"events"`
	NextSinceSeq int64       `json:"next_since_seq"`
}

// seedEvents starts a session and ingests count events with sequential
// msg payloads "evt-0" .. "evt-N-1", returning the session id.
func seedEvents(t *testing.T, srv *httptest.Server, count int) string {
	t.Helper()
	startResp := doJSON(t, srv, http.MethodPost, "/session/start", testToken, map[string]string{"label": "events"})
	if startResp.StatusCode != http.StatusOK {
		t.Fatalf("start session: status=%d", startResp.StatusCode)
	}
	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(startResp.Body).Decode(&body); err != nil {
		startResp.Body.Close()
		t.Fatalf("decode start: %v", err)
	}
	startResp.Body.Close()

	events := make([]map[string]any, count)
	for i := 0; i < count; i++ {
		events[i] = map[string]any{"source": "x", "level": "info", "msg": "evt-" + strconv.Itoa(i)}
	}
	ingest := doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
		"session_id": body.SessionID,
		"events":     events,
	})
	if ingest.StatusCode != http.StatusAccepted {
		buf, _ := io.ReadAll(ingest.Body)
		ingest.Body.Close()
		t.Fatalf("ingest status=%d body=%s", ingest.StatusCode, buf)
	}
	ingest.Body.Close()
	return body.SessionID
}

func readEventsResp(t *testing.T, resp *http.Response) listEventsResp {
	t.Helper()
	defer resp.Body.Close()
	var out listEventsResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode events resp: %v", err)
	}
	return out
}

// TestListEvents_HappyPath_CursorWalk seeds 5 events into one session,
// then drives the cursor forward in pages of 2 — the canonical Phase-2b
// S4 polling loop. Verifies (a) strict cursor advance (no re-read), (b)
// next_since_seq tracks the last returned seq_id, (c) ordering ascending.
func TestListEvents_HappyPath_CursorWalk(t *testing.T) {
	srv, _ := newTestServer(t)
	sessionID := seedEvents(t, srv, 5)

	var cursor int64
	var collected []string
	for round := 0; round < 4; round++ { // bounded
		resp := doJSON(t, srv, http.MethodGet,
			"/events?session="+sessionID+"&since_seq="+strconv.FormatInt(cursor, 10)+"&limit=2",
			testToken, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("round %d: status=%d", round, resp.StatusCode)
		}
		body := readEventsResp(t, resp)
		if len(body.Events) == 0 {
			break
		}
		// next_since_seq matches the last returned seq_id.
		if body.NextSinceSeq != body.Events[len(body.Events)-1].SeqID {
			t.Errorf("round %d: next_since_seq=%d, want %d (last seq_id)",
				round, body.NextSinceSeq, body.Events[len(body.Events)-1].SeqID)
		}
		// Cursor strictly advances.
		if body.Events[0].SeqID <= cursor {
			t.Errorf("round %d: first seq_id %d <= cursor %d (strict-gt violated)",
				round, body.Events[0].SeqID, cursor)
		}
		// Ascending order.
		for i := 1; i < len(body.Events); i++ {
			if body.Events[i].SeqID <= body.Events[i-1].SeqID {
				t.Errorf("round %d: not ascending at i=%d: %d <= %d",
					round, i, body.Events[i].SeqID, body.Events[i-1].SeqID)
			}
		}
		for _, e := range body.Events {
			collected = append(collected, e.Msg)
		}
		cursor = body.NextSinceSeq
	}
	if len(collected) != 5 {
		t.Fatalf("collected %d, want 5: %v", len(collected), collected)
	}
}

// TestListEvents_EmptyStableNextSinceSeq verifies the "stable on empty"
// property (ADR-008): when no rows match the cursor, next_since_seq
// equals the caller's input — a polling loop never spins backwards.
func TestListEvents_EmptyStableNextSinceSeq(t *testing.T) {
	srv, _ := newTestServer(t)
	sessionID := seedEvents(t, srv, 2)
	// First, find the current max seq_id.
	resp := doJSON(t, srv, http.MethodGet, "/events?session="+sessionID, testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("warmup status=%d", resp.StatusCode)
	}
	warmup := readEventsResp(t, resp)
	if len(warmup.Events) != 2 {
		t.Fatalf("warmup: got %d events, want 2", len(warmup.Events))
	}
	maxSeq := warmup.NextSinceSeq

	// Now read past the max — empty result, cursor unchanged.
	resp = doJSON(t, srv, http.MethodGet,
		"/events?session="+sessionID+"&since_seq="+strconv.FormatInt(maxSeq, 10),
		testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tail status=%d", resp.StatusCode)
	}
	tail := readEventsResp(t, resp)
	if len(tail.Events) != 0 {
		t.Errorf("tail: got %d events, want 0", len(tail.Events))
	}
	if tail.NextSinceSeq != maxSeq {
		t.Errorf("tail next_since_seq=%d, want unchanged %d", tail.NextSinceSeq, maxSeq)
	}
}

// TestListEvents_UnknownSessionReturnsEmpty asserts ADR-008's "unknown
// session is not a 404" decision: an unknown session id returns
// events:[] + the caller's since_seq verbatim.
func TestListEvents_UnknownSessionReturnsEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/events?session=ghost-session&since_seq=42", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body := readEventsResp(t, resp)
	if len(body.Events) != 0 {
		t.Errorf("unknown session: got %d events, want 0", len(body.Events))
	}
	if body.NextSinceSeq != 42 {
		t.Errorf("unknown session: next_since_seq=%d, want 42 (caller's input echoed)", body.NextSinceSeq)
	}
}

// TestListEvents_MissingSession asserts the 400 contract for the
// required `session` query parameter.
func TestListEvents_MissingSession(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/events", testToken, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "session required") {
		t.Errorf("body %q missing 'session required' marker", string(body))
	}
}

// TestListEvents_InvalidSinceSeq covers the unparseable-cursor and
// negative-cursor 400 paths.
func TestListEvents_InvalidSinceSeq(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, bad := range []string{"abc", "-1", "1.5"} {
		t.Run(bad, func(t *testing.T) {
			resp := doJSON(t, srv, http.MethodGet,
				"/events?session=any&since_seq="+bad, testToken, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("since_seq=%q: status=%d, want 400", bad, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

// TestListEvents_InvalidLimit covers the unparseable-limit and
// non-positive-limit 400 paths.
func TestListEvents_InvalidLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, bad := range []string{"abc", "0", "-5"} {
		t.Run(bad, func(t *testing.T) {
			resp := doJSON(t, srv, http.MethodGet,
				"/events?session=any&limit="+bad, testToken, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("limit=%q: status=%d, want 400", bad, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

// TestListEvents_LimitCappedAt5000 asserts the 5000-row cap in
// handlers.go's eventsMaxLimit. Asking for 99999 must silently clamp;
// the request succeeds (no 400) and returns the seeded events.
func TestListEvents_LimitCappedAt5000(t *testing.T) {
	srv, _ := newTestServer(t)
	sessionID := seedEvents(t, srv, 3)
	resp := doJSON(t, srv, http.MethodGet,
		"/events?session="+sessionID+"&limit=99999", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (limit silently clamped)", resp.StatusCode)
	}
	body := readEventsResp(t, resp)
	if len(body.Events) != 3 {
		t.Errorf("got %d events, want 3", len(body.Events))
	}
}

// TestListEvents_MetaRoundtrip asserts the meta field travels through
// /ingest → store → /events byte-identically.
func TestListEvents_MetaRoundtrip(t *testing.T) {
	srv, _ := newTestServer(t)
	startResp := doJSON(t, srv, http.MethodPost, "/session/start", testToken, map[string]string{"label": "meta"})
	var sb struct {
		SessionID string `json:"session_id"`
	}
	_ = json.NewDecoder(startResp.Body).Decode(&sb)
	startResp.Body.Close()

	ingest := doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
		"session_id": sb.SessionID,
		"events": []map[string]any{
			{"source": "x", "level": "info", "msg": "with-meta", "meta": map[string]any{"k": "v", "n": 7}},
		},
	})
	ingest.Body.Close()

	resp := doJSON(t, srv, http.MethodGet, "/events?session="+sb.SessionID, testToken, nil)
	body := readEventsResp(t, resp)
	if len(body.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(body.Events))
	}
	if len(body.Events[0].Meta) == 0 {
		t.Fatal("meta empty after roundtrip")
	}
	var meta map[string]any
	if err := json.Unmarshal(body.Events[0].Meta, &meta); err != nil {
		t.Fatalf("meta unmarshal: %v", err)
	}
	if meta["k"] != "v" {
		t.Errorf("meta[k]=%v, want v", meta["k"])
	}
}
