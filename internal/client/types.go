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
//   - SeqID is populated only on /events responses (Phase 2b S4,
//     ADR-008): it is the opaque int64 cursor consumers feed back as
//     the next call's since_seq. `omitempty` keeps /ingest + /tail
//     wire-identical to pre-S4 traffic (no rolling-upgrade drift).
type Event struct {
	SeqID     int64          `json:"seq_id,omitempty"`
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

// CrashEvent is the wire shape returned by GET /crashes (Phase 2b S6,
// ADR-009). Mirrors store.CrashRow but lives here so the client package
// owns its own public DTO (no import of internal/store).
//
// Field semantics:
//
//   - ID is the SQLite ROWID of the crash row, useful as an opaque
//     pin for future per-crash follow-up. `omitempty` keeps payloads
//     compact when the consumer does not rely on it.
//   - SessionID echoes the requested session. `omitempty` mirrors the
//     Event DTO so consumers that already know the session can ignore
//     the field.
//   - TS is unix-nanoseconds (same envelope as Event.TS), set to the
//     most-recent occurrence per the dedup-upsert in
//     store.UpsertCrash.
//   - Fingerprint is the SHA256-top-3-frames hash from P1-S5 (hex-16).
//   - Stacktrace is the raw stack body as captured at first detection
//     (subsequent dedup hits do not overwrite the body — first insert
//     wins).
//   - Count is the dedup counter: occurrences of the same
//     (session_id, fingerprint) pair collapsed into one row.
type CrashEvent struct {
	ID          int64  `json:"id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	TS          int64  `json:"ts"`
	Fingerprint string `json:"fingerprint"`
	Stacktrace  string `json:"stacktrace"`
	Count       int    `json:"count"`
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

// eventsSinceEventWire is the on-wire shape of one /events response row.
// Mirrors internal/http.eventView; kept here so the client package owns
// its own decoder type (no import of internal/http).
type eventsSinceEventWire struct {
	SeqID     int64           `json:"seq_id"`
	SessionID string          `json:"session_id"`
	TS        int64           `json:"ts"`
	Source    string          `json:"source"`
	Level     string          `json:"level"`
	Msg       string          `json:"msg"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

type eventsSinceRespWire struct {
	Events       []eventsSinceEventWire `json:"events"`
	NextSinceSeq int64                  `json:"next_since_seq"`
}

// crashesListRespWire is the on-wire shape of the /crashes response.
// Mirrors internal/http.listCrashesResp; kept here so the client owns
// its own decoder type (no import of internal/http).
type crashesListRespWire struct {
	Crashes []CrashEvent `json:"crashes"`
}
