package client

import "encoding/json"

// Event is the bidirectional event shape used by both POST /ingest
// (request-side, see Ingest) and GET /tail (response-side, see Tail).
// It mirrors the hub's internal/http.ingestEvent + internal/ws.Event
// types but lives here so the client does not depend on any hub-internal
// package.
//
// Field semantics:
//
//   - TS is unix-nanoseconds; the hub fills in time.Now() when 0 is sent
//     on /ingest and always populates it on /tail frames.
//   - Source, Level, Msg are plain strings (level is free-form; the CLI
//     colour-mapper recognises ERROR/WARN/INFO/DEBUG case-insensitively).
//   - Meta is opaque JSON — typed as map[string]any for ergonomic call
//     sites; the wire format is identical to json.RawMessage on the hub.
//   - SessionID is empty on the ingest path (the hub knows the session
//     from the POST envelope) and populated on /tail frames so a single
//     unfiltered subscriber can demultiplex multiple sessions.
type Event struct {
	SessionID string         `json:"session_id,omitempty"`
	TS        int64          `json:"ts,omitempty"`
	Source    string         `json:"source"`
	Level     string         `json:"level"`
	Msg       string         `json:"msg"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// Session mirrors the hub's sessionView (internal/http/handlers.go). The
// hub serialises EndedAt as a nullable JSON field (`ended_at,omitempty`
// on a `*int64`), which we mirror with a pointer so nil means "still
// running".
//
// StartedAt and EndedAt are unix-nanoseconds.
type Session struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	StartedAt int64  `json:"started_at"`
	EndedAt   *int64 `json:"ended_at,omitempty"`
}

// --- internal wire types (not exported) ---

type sessionStartReqWire struct {
	Label string `json:"label"`
}

type sessionStartRespWire struct {
	SessionID string `json:"session_id"`
	StartedAt int64  `json:"started_at"`
}

type sessionEndReqWire struct {
	SessionID string `json:"session_id"`
}

// ingestEventWire is the on-wire event shape. We re-encode Meta as
// json.RawMessage so the hub's DisallowUnknownFields decoder accepts our
// payload byte-for-byte identical to what /ingest expects internally.
type ingestEventWire struct {
	TS     int64           `json:"ts,omitempty"`
	Source string          `json:"source"`
	Level  string          `json:"level"`
	Msg    string          `json:"msg"`
	Meta   json.RawMessage `json:"meta,omitempty"`
}

type ingestReqWire struct {
	SessionID string            `json:"session_id"`
	Events    []ingestEventWire `json:"events"`
}

type ingestRespWire struct {
	Ingested int `json:"ingested"`
}

type listSessionsRespWire struct {
	Sessions []Session `json:"sessions"`
}
