package http_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
)

const javaStackMsg = `Exception in thread "main" java.lang.NullPointerException: Cannot invoke "String.length()" because "x" is null
	at com.example.foo.Bar.doStuff(Bar.java:42)
	at com.example.foo.Bar.main(Bar.java:17)
	at jdk.internal.reflect.NativeMethodAccessorImpl.invoke0(NativeMethodAccessorImpl.java:-2)`

func TestIngestStacktraceCreatesCrashRow(t *testing.T) {
	srv, st := newTestServer(t)

	// Start session.
	resp := doJSON(t, srv, http.MethodPost, "/session/start", testToken,
		map[string]string{"label": "crash-create"})
	var startBody struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&startBody); err != nil {
		resp.Body.Close()
		t.Fatalf("decode start: %v", err)
	}
	resp.Body.Close()

	// Ingest event WITH a Java stack.
	resp = doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
		"session_id": startBody.SessionID,
		"events": []map[string]any{
			{"source": "logcat", "level": "ERROR", "msg": javaStackMsg},
		},
	})
	if resp.StatusCode != http.StatusAccepted {
		buf, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ingest status=%d body=%s", resp.StatusCode, buf)
	}
	resp.Body.Close()

	crashes, err := st.CrashesBySession(t.Context(), startBody.SessionID)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(crashes) != 1 {
		t.Fatalf("want 1 crash row, got %d", len(crashes))
	}
	if crashes[0].Count != 1 {
		t.Errorf("count = %d, want 1", crashes[0].Count)
	}
	if crashes[0].Fingerprint == "" {
		t.Error("empty fingerprint")
	}
}

func TestIngestNoStacktraceCreatesNoCrashRow(t *testing.T) {
	srv, st := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/session/start", testToken,
		map[string]string{"label": "no-crash"})
	var startBody struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&startBody); err != nil {
		resp.Body.Close()
		t.Fatalf("decode start: %v", err)
	}
	resp.Body.Close()

	// Ingest two normal events — must not create any crash rows.
	resp = doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
		"session_id": startBody.SessionID,
		"events": []map[string]any{
			{"source": "logcat", "level": "INFO", "msg": "request completed in 42ms"},
			{"source": "logcat", "level": "WARN", "msg": "cache miss for key=foo"},
		},
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("ingest status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	crashes, err := st.CrashesBySession(t.Context(), startBody.SessionID)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(crashes) != 0 {
		t.Fatalf("want 0 crash rows, got %d: %+v", len(crashes), crashes)
	}
}

func TestIngestStacktraceDedupCounter(t *testing.T) {
	srv, st := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/session/start", testToken,
		map[string]string{"label": "dedup"})
	var startBody struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&startBody); err != nil {
		resp.Body.Close()
		t.Fatalf("decode start: %v", err)
	}
	resp.Body.Close()

	// Three separate /ingest calls with the exact same stack.
	for i := 0; i < 3; i++ {
		resp = doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
			"session_id": startBody.SessionID,
			"events": []map[string]any{
				{"source": "logcat", "level": "ERROR", "msg": javaStackMsg},
			},
		})
		if resp.StatusCode != http.StatusAccepted {
			buf, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("ingest #%d status=%d body=%s", i, resp.StatusCode, buf)
		}
		resp.Body.Close()
	}

	crashes, err := st.CrashesBySession(t.Context(), startBody.SessionID)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(crashes) != 1 {
		t.Fatalf("want 1 deduped crash row, got %d: %+v", len(crashes), crashes)
	}
	if crashes[0].Count != 3 {
		t.Errorf("count = %d, want 3", crashes[0].Count)
	}
}

// TestIngestStacktraceAfterSessionEndStillRecorded documents the
// best-effort crash-upsert behaviour from handlers.go::ingest: a crash
// arriving after /session/end is informational — the session row still
// exists, the FK still resolves, and the crash row is written.
//
// Documented behaviour we cannot reproduce here without a mock: if
// store.UpsertCrash itself fails, handlers.detectAndUpsertCrashes logs
// the error and continues; the /ingest response is unaffected (the row
// has already been written to events). Reproducing that failure path
// requires a Store-Interface mock — see M5 follow-up (qs-20260510-003).
//
// Pattern choice: we kept the smoke-test variant rather than extracting
// a Store interface. Today handlers.handlers holds *store.Store
// directly; extracting an interface for a single failure-injection test
// would ripple through server.go::New and the *store.Store call sites
// (CreateSession, EndSession, InsertEvents, UpsertCrash, ListSessions,
// CrashesBySession, RecentEvents — 7 methods). The benefit/cost ratio
// doesn't justify it for a "logs and continues" path that has zero
// production callers depending on the error surfacing. If a future
// finding promotes upsert-failure to a hard 5xx or a retry path, the
// interface extraction becomes worthwhile and is scoped as M5-follow-up.
func TestIngestStacktraceAfterSessionEndStillRecorded(t *testing.T) {
	srv, st := newTestServer(t)

	resp := doJSON(t, srv, http.MethodPost, "/session/start", testToken,
		map[string]string{"label": "ended-then-ingest"})
	var startBody struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&startBody); err != nil {
		resp.Body.Close()
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	// End the session. Subsequent /ingest still works (sessions don't
	// hard-stop ingest, ended_at is informational), so the crash-upsert
	// path runs against a live session.
	resp = doJSON(t, srv, http.MethodPost, "/session/end", testToken,
		map[string]string{"session_id": startBody.SessionID})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("end status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doJSON(t, srv, http.MethodPost, "/ingest", testToken, map[string]any{
		"session_id": startBody.SessionID,
		"events": []map[string]any{
			{"source": "logcat", "level": "ERROR", "msg": javaStackMsg},
		},
	})
	if resp.StatusCode != http.StatusAccepted {
		buf, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("ingest after end: status=%d body=%s", resp.StatusCode, buf)
	}
	resp.Body.Close()

	// Crash should still be recorded.
	var crashes []store.CrashRow
	crashes, err := st.CrashesBySession(t.Context(), startBody.SessionID)
	if err != nil {
		t.Fatalf("CrashesBySession: %v", err)
	}
	if len(crashes) != 1 {
		t.Fatalf("want 1 crash row, got %d", len(crashes))
	}
}
