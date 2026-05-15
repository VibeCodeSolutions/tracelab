package http_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

// crashView mirrors the wire shape of a /crashes response row. Kept
// local to the test so an accidental rename in handlers.go's crashView
// fails this test rather than silently passing.
type crashView struct {
	ID          int64  `json:"id"`
	SessionID   string `json:"session_id"`
	TS          int64  `json:"ts"`
	Fingerprint string `json:"fingerprint"`
	Stacktrace  string `json:"stacktrace"`
	Count       int    `json:"count"`
}

type listCrashesResp struct {
	Crashes []crashView `json:"crashes"`
}

// seedCrashes starts a session and upserts count distinct crashes with
// strictly increasing ts. Returns the session id.
func seedCrashes(t *testing.T, srv *httptest.Server, st *store.Store, count int) string {
	t.Helper()
	startResp := doJSON(t, srv, http.MethodPost, "/session/start", testToken, map[string]string{"label": "crashes"})
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

	// Drive crashes through the store directly: /ingest with a
	// stacktrace would invoke the detector, but we want deterministic
	// fingerprints + counts. The handler under test reads from the
	// store, so this is the cleanest seed path.
	for i := 0; i < count; i++ {
		ts := time.Unix(int64(1700000000+i), 0)
		fp := "fp-test-" + string(rune('a'+i))
		if err := st.UpsertCrash(t.Context(), body.SessionID, ts, fp, "stack "+fp); err != nil {
			t.Fatalf("UpsertCrash %d: %v", i, err)
		}
	}
	return body.SessionID
}

func readCrashesResp(t *testing.T, resp *http.Response) listCrashesResp {
	t.Helper()
	defer resp.Body.Close()
	var out listCrashesResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode crashes resp: %v", err)
	}
	return out
}

// TestListCrashes_HappyPath seeds three crashes and verifies the hub
// returns them newest-first with the full row shape (id, session_id,
// ts, fingerprint, stacktrace, count).
func TestListCrashes_HappyPath(t *testing.T) {
	srv, st := newTestServer(t)
	sessionID := seedCrashes(t, srv, st, 3)

	resp := doJSON(t, srv, http.MethodGet, "/crashes?session="+sessionID, testToken, nil)
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status=%d body=%s", resp.StatusCode, buf)
	}
	body := readCrashesResp(t, resp)
	if len(body.Crashes) != 3 {
		t.Fatalf("got %d crashes, want 3", len(body.Crashes))
	}
	// Newest first: ts=...002, ...001, ...000.
	if body.Crashes[0].Fingerprint != "fp-test-c" {
		t.Errorf("first fp = %q, want fp-test-c (newest)", body.Crashes[0].Fingerprint)
	}
	if body.Crashes[2].Fingerprint != "fp-test-a" {
		t.Errorf("last fp = %q, want fp-test-a (oldest)", body.Crashes[2].Fingerprint)
	}
	// Sanity-check ordering monotonicity (ts desc).
	for i := 1; i < len(body.Crashes); i++ {
		if body.Crashes[i].TS > body.Crashes[i-1].TS {
			t.Errorf("not descending at i=%d: ts %d > %d",
				i, body.Crashes[i].TS, body.Crashes[i-1].TS)
		}
	}
	// Session-id roundtrip + count = 1 (no dedup-hits).
	for _, c := range body.Crashes {
		if c.SessionID != sessionID {
			t.Errorf("session_id leak: got %q, want %q", c.SessionID, sessionID)
		}
		if c.Count != 1 {
			t.Errorf("count = %d, want 1", c.Count)
		}
		if c.ID == 0 {
			t.Errorf("id = 0, want non-zero")
		}
	}
}

// TestListCrashes_LimitClamps asserts the silent-cap behaviour at
// crashesMaxLimit. limit=99999 silently clamps; the request succeeds.
func TestListCrashes_LimitClamps(t *testing.T) {
	srv, st := newTestServer(t)
	sessionID := seedCrashes(t, srv, st, 3)
	resp := doJSON(t, srv, http.MethodGet,
		"/crashes?session="+sessionID+"&limit=99999", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (limit silently clamped)", resp.StatusCode)
	}
	body := readCrashesResp(t, resp)
	if len(body.Crashes) != 3 {
		t.Errorf("got %d, want 3", len(body.Crashes))
	}
}

// TestListCrashes_LimitTrims asserts a small limit trims the result to
// the newest N rows.
func TestListCrashes_LimitTrims(t *testing.T) {
	srv, st := newTestServer(t)
	sessionID := seedCrashes(t, srv, st, 5)
	resp := doJSON(t, srv, http.MethodGet,
		"/crashes?session="+sessionID+"&limit=2", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body := readCrashesResp(t, resp)
	if len(body.Crashes) != 2 {
		t.Fatalf("got %d, want 2", len(body.Crashes))
	}
	// Newest first: fp-test-e (ts=...004) then fp-test-d.
	if body.Crashes[0].Fingerprint != "fp-test-e" || body.Crashes[1].Fingerprint != "fp-test-d" {
		t.Errorf("got %s/%s, want fp-test-e/fp-test-d (newest first)",
			body.Crashes[0].Fingerprint, body.Crashes[1].Fingerprint)
	}
}

// TestListCrashes_UnknownSessionReturnsEmpty asserts ADR-009's "unknown
// session is not a 404" decision: an unknown session id returns
// crashes:[].
func TestListCrashes_UnknownSessionReturnsEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/crashes?session=ghost-session", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	body := readCrashesResp(t, resp)
	if len(body.Crashes) != 0 {
		t.Errorf("unknown session: got %d crashes, want 0", len(body.Crashes))
	}
}

// TestListCrashes_MissingSession asserts the 400 contract for the
// required `session` query parameter.
func TestListCrashes_MissingSession(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/crashes", testToken, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "session required") {
		t.Errorf("body %q missing 'session required' marker", string(body))
	}
}

// TestListCrashes_InvalidLimit covers the unparseable-limit and
// non-positive-limit 400 paths.
func TestListCrashes_InvalidLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, bad := range []string{"abc", "0", "-5"} {
		t.Run(bad, func(t *testing.T) {
			resp := doJSON(t, srv, http.MethodGet,
				"/crashes?session=any&limit="+bad, testToken, nil)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("limit=%q: status=%d, want 400", bad, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

// TestListCrashes_DedupCountSurfaces asserts the dedup count from
// repeated UpsertCrash calls travels through the wire correctly — the
// same (session, fingerprint) pair bumped three times must surface as
// count:3.
func TestListCrashes_DedupCountSurfaces(t *testing.T) {
	srv, st := newTestServer(t)
	startResp := doJSON(t, srv, http.MethodPost, "/session/start", testToken, map[string]string{"label": "dedup"})
	var sb struct {
		SessionID string `json:"session_id"`
	}
	_ = json.NewDecoder(startResp.Body).Decode(&sb)
	startResp.Body.Close()

	for i := 0; i < 3; i++ {
		ts := time.Unix(int64(1700000000+i), 0)
		if err := st.UpsertCrash(t.Context(), sb.SessionID, ts, "fp-dedup", "stack v"+string(rune('0'+i))); err != nil {
			t.Fatalf("UpsertCrash %d: %v", i, err)
		}
	}

	resp := doJSON(t, srv, http.MethodGet, "/crashes?session="+sb.SessionID, testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body := readCrashesResp(t, resp)
	if len(body.Crashes) != 1 {
		t.Fatalf("got %d crashes, want 1 (dedup)", len(body.Crashes))
	}
	if body.Crashes[0].Count != 3 {
		t.Errorf("count = %d, want 3", body.Crashes[0].Count)
	}
}
