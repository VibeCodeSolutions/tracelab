package client

import "encoding/json"

// Event is the request-side event shape sent to POST /ingest. It mirrors
// the hub's internal/http.ingestEvent type (see internal/http/handlers.go)
// but lives here so the client does not depend on any hub-internal
// package.
//
// TS is unix-nanoseconds; the hub fills in time.Now() when 0 is sent.
// Meta is opaque JSON — typed as map[string]any for ergonomic call sites;
// the wire format is identical to json.RawMessage on the hub side.
type Event struct {
	TS     int64          `json:"ts,omitempty"`
	Source string         `json:"source"`
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Meta   map[string]any `json:"meta,omitempty"`
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
