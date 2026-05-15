package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/crash"
	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// handlers groups the per-route handler funcs with their dependencies so
// they can be wired in server.go without package-level state.
type handlers struct {
	store *store.Store
	hub   *ws.Hub
	log   *slog.Logger
}

// writeJSON serialises v to w with the application/json content type and
// the given HTTP status. Encoding errors are logged via slog.Default()
// because at that point the response headers are already flushed.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Default().Error("writeJSON encode failed", slog.Any("error", err))
	}
}

// decodeJSON reads and parses the request body into v, returning a 400 on
// failure. On success, the body is closed before returning.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return false
	}
	return true
}

// internalError logs the underlying error and writes a 500 with a generic
// body — never leak err.Error() to the client.
func (h *handlers) internalError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	h.log.LogAttrs(r.Context(), slog.LevelError, msg, slog.Any("error", err))
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
}

// healthz is the unauthenticated liveness probe.
func (h *handlers) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type sessionStartReq struct {
	Label string `json:"label"`
}

type sessionStartResp struct {
	SessionID string `json:"session_id"`
	StartedAt int64  `json:"started_at"`
}

func (h *handlers) sessionStart(w http.ResponseWriter, r *http.Request) {
	var req sessionStartReq
	if !decodeJSON(w, r, &req) {
		return
	}
	id, err := h.store.CreateSession(r.Context(), req.Label)
	if err != nil {
		h.internalError(w, r, "session start failed", err)
		return
	}
	writeJSON(w, http.StatusOK, sessionStartResp{
		SessionID: id,
		StartedAt: time.Now().UnixNano(),
	})
}

type sessionEndReq struct {
	SessionID string `json:"session_id"`
}

func (h *handlers) sessionEnd(w http.ResponseWriter, r *http.Request) {
	var req sessionEndReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id required"})
		return
	}
	if err := h.store.EndSession(r.Context(), req.SessionID); err != nil {
		// Distinguish unknown session (404) from real DB problems (500).
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		h.internalError(w, r, "session end failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type ingestEvent struct {
	TS     int64           `json:"ts"`
	Source string          `json:"source"`
	Level  string          `json:"level"`
	Msg    string          `json:"msg"`
	Meta   json.RawMessage `json:"meta,omitempty"`
}

type ingestReq struct {
	SessionID string        `json:"session_id"`
	Events    []ingestEvent `json:"events"`
}

type ingestResp struct {
	Ingested int `json:"ingested"`
}

func (h *handlers) ingest(w http.ResponseWriter, r *http.Request) {
	var req ingestReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id required"})
		return
	}
	if len(req.Events) == 0 {
		writeJSON(w, http.StatusAccepted, ingestResp{Ingested: 0})
		return
	}
	batch := make([]store.Event, len(req.Events))
	for i, e := range req.Events {
		var ts time.Time
		if e.TS > 0 {
			ts = time.Unix(0, e.TS)
		}
		batch[i] = store.Event{
			TS:     ts,
			Source: e.Source,
			Level:  e.Level,
			Msg:    e.Msg,
			Meta:   e.Meta,
		}
	}
	if err := h.store.InsertEvents(r.Context(), req.SessionID, batch); err != nil {
		h.internalError(w, r, "ingest failed", err)
		return
	}
	// Fan out to /tail subscribers after a successful DB insert. The hub
	// performs non-blocking sends, so a slow WS client never stalls ingest.
	if h.hub != nil {
		for _, e := range batch {
			h.hub.Publish(ws.Event{
				SessionID: req.SessionID,
				TS:        e.TS.UnixNano(),
				Source:    e.Source,
				Level:     e.Level,
				Msg:       e.Msg,
				Meta:      e.Meta,
			})
		}
	}
	// Run stacktrace detection on each event. Crash-row upserts are
	// best-effort: a failure here is logged but does NOT change the
	// /ingest response — the events themselves are already durably
	// persisted, and missing crash-table entries are recoverable from the
	// raw event log.
	h.detectAndUpsertCrashes(r.Context(), req.SessionID, batch)

	writeJSON(w, http.StatusAccepted, ingestResp{Ingested: len(batch)})
}

// detectAndUpsertCrashes scans each event for a stacktrace and, on match,
// upserts a row into the crashes table. Errors are logged and swallowed
// to keep /ingest 202-clean (see ADR-006).
func (h *handlers) detectAndUpsertCrashes(ctx context.Context, sessionID string, batch []store.Event) {
	for _, e := range batch {
		res := crash.Detect(e.Source, e.Level, e.Msg, nil)
		if !res.Matched {
			continue
		}
		fp := crash.Fingerprint(res.NormalizedStack)
		if fp == "" {
			continue
		}
		ts := e.TS
		if ts.IsZero() {
			ts = time.Now()
		}
		if err := h.store.UpsertCrash(ctx, sessionID, ts, fp, e.Msg); err != nil {
			h.log.LogAttrs(ctx, slog.LevelWarn, "crash upsert failed",
				slog.String("session_id", sessionID),
				slog.String("language", string(res.Language)),
				slog.String("fingerprint", fp),
				slog.Any("error", err),
			)
			continue
		}
		h.log.LogAttrs(ctx, slog.LevelInfo, "crash detected",
			slog.String("session_id", sessionID),
			slog.String("language", string(res.Language)),
			slog.String("fingerprint", fp),
		)
	}
}

// eventsDefaultLimit / eventsMaxLimit cap GET /events response sizes
// (ADR-008 Decision 4). The default trades round-trip count against
// payload size; the cap keeps a single stdio-MCP frame well under
// transport buffer limits even with verbose `meta` payloads.
const (
	eventsDefaultLimit = 500
	eventsMaxLimit     = 5000
)

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

// listEvents serves GET /events?session=<id>&since_seq=<n>&limit=<n>.
// See ADR-008: opaque events.id cursor, strict `id > since_seq`, stable
// next_since_seq on empty results (caller's input is echoed back so a
// polling loop never spins on a stale cursor).
//
// Auth is enforced by the bearer-protected sub-router in server.go.
// Unknown session id is NOT a 404 — it returns events:[] + the caller's
// since_seq verbatim (the endpoint is a forward-cursor read, not a
// session-existence probe; existence is discoverable via /sessions).
func (h *handlers) listEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sessionID := q.Get("session")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session required"})
		return
	}

	var sinceSeq int64
	if raw := q.Get("since_seq"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid since_seq: " + raw,
			})
			return
		}
		sinceSeq = n
	}

	limit := eventsDefaultLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid limit: " + raw,
			})
			return
		}
		if n > eventsMaxLimit {
			n = eventsMaxLimit
		}
		limit = n
	}

	events, err := h.store.EventsSince(r.Context(), sessionID, sinceSeq, limit)
	if err != nil {
		h.internalError(w, r, "list events failed", err)
		return
	}

	out := make([]eventView, len(events))
	nextSinceSeq := sinceSeq // stable-on-empty: echo caller's cursor
	for i, e := range events {
		out[i] = eventView{
			SeqID:     e.ID,
			SessionID: e.SessionID,
			TS:        e.TS.UnixNano(),
			Source:    e.Source,
			Level:     e.Level,
			Msg:       e.Msg,
			Meta:      e.Meta,
		}
		if e.ID > nextSinceSeq {
			nextSinceSeq = e.ID
		}
	}
	writeJSON(w, http.StatusOK, listEventsResp{
		Events:       out,
		NextSinceSeq: nextSinceSeq,
	})
}

type sessionView struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	StartedAt int64  `json:"started_at"`
	EndedAt   *int64 `json:"ended_at,omitempty"`
}

type listSessionsResp struct {
	Sessions []sessionView `json:"sessions"`
}

func (h *handlers) listSessions(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	sessions, err := h.store.ListSessions(r.Context(), limit)
	if err != nil {
		h.internalError(w, r, "list sessions failed", err)
		return
	}
	out := make([]sessionView, len(sessions))
	for i, s := range sessions {
		v := sessionView{
			ID:        s.ID,
			Label:     s.Label,
			StartedAt: s.StartedAt.UnixNano(),
		}
		if s.EndedAt != nil {
			n := s.EndedAt.UnixNano()
			v.EndedAt = &n
		}
		out[i] = v
	}
	writeJSON(w, http.StatusOK, listSessionsResp{Sessions: out})
}
