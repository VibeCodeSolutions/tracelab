// Phase 2b S4 — events.go owns the forward-cursor read against the
// hub's GET /events endpoint (ADR-008). It is the single consumer
// surface for the MCP tail_since tool.
//
// Semantics in one paragraph:
//
//   - Cursor (since_seq, next_since_seq) is opaque int64; values map
//     1:1 to the hub's events.id and are strictly forward-only.
//   - On empty results the hub echoes the caller's since_seq back as
//     next_since_seq, so a polling loop never spins on a stale cursor.
//   - Unknown session id is NOT a 404 — it returns events:[] +
//     since_seq verbatim (the endpoint is a cursor read, not a session
//     probe).
//   - Bearer auth and HTTPError sentinels (ErrUnauthorized /
//     ErrServerError) are the same as every other authenticated client
//     method; no /events-specific error shape.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// EventsSince calls the hub's GET /events endpoint with the given
// cursor and returns the next page of events plus the cursor to use on
// the next call.
//
//   - session must be non-empty; an empty value triggers a client-side
//     error (no network round-trip).
//   - sinceSeq <= 0 is treated as "start from the earliest event" (the
//     query parameter is omitted; the hub then defaults to since_seq=0).
//   - limit <= 0 is treated as "use the hub default" (currently 500).
//     The hub caps at 5000; values above are silently clamped.
//
// The returned events are sorted ascending by SeqID. nextSinceSeq is
// the maximum SeqID returned, or sinceSeq when the result is empty
// ("stable on empty" — see ADR-008).
func (c *Client) EventsSince(ctx context.Context, session string, sinceSeq int64, limit int) (events []Event, nextSinceSeq int64, err error) {
	if session == "" {
		return nil, 0, errors.New("client: EventsSince requires a non-empty session")
	}

	q := url.Values{}
	q.Set("session", session)
	if sinceSeq > 0 {
		q.Set("since_seq", strconv.FormatInt(sinceSeq, 10))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp eventsSinceRespWire
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     "/events?" + q.Encode(),
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return nil, 0, err
	}

	// Decode the wire rows into the public Event type. We preserve
	// Meta as map[string]any (the public shape) by re-unmarshalling
	// the RawMessage; on absent/null meta we leave the field nil.
	out := make([]Event, len(resp.Events))
	for i, w := range resp.Events {
		out[i] = Event{
			SeqID:     w.SeqID,
			SessionID: w.SessionID,
			TS:        w.TS,
			Source:    w.Source,
			Level:     w.Level,
			Msg:       w.Msg,
		}
		if len(w.Meta) > 0 && string(w.Meta) != "null" {
			var m map[string]any
			if err := json.Unmarshal(w.Meta, &m); err == nil {
				out[i].Meta = m
			}
		}
	}
	return out, resp.NextSinceSeq, nil
}
