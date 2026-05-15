// Phase 2b S6 — crashes.go owns the read against the hub's
// GET /crashes endpoint (ADR-009). It is the single consumer
// surface for the MCP crashes_list tool.
//
// Semantics in one paragraph:
//
//   - "List crashes for a session, newest first, capped at limit."
//     Unlike /events this is NOT a forward cursor — callers fetch the
//     digest once per triage, they do not poll forward.
//   - limit <= 0 means "use the hub default" (currently 500). The hub
//     caps at 5000; values above are silently clamped.
//   - Unknown session id is NOT a 404 — it returns crashes:[] (the
//     endpoint is a list-read, not a session-existence probe; existence
//     is discoverable via /sessions).
//   - Bearer auth and HTTPError sentinels (ErrUnauthorized /
//     ErrServerError) are the same as every other authenticated client
//     method; no /crashes-specific error shape.

package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// CrashesList calls the hub's GET /crashes endpoint and returns the
// session's crash digest, newest first.
//
//   - session must be non-empty; an empty value triggers a client-side
//     error (no network round-trip).
//   - limit <= 0 is treated as "use the hub default" (currently 500);
//     the query parameter is omitted. The hub caps at 5000; values
//     above are silently clamped.
//
// The returned slice is sorted descending by ts (and id as tiebreaker)
// — same envelope the store-side CrashesBySession produces. A nil
// result from the hub is normalised to an empty slice so callers never
// need to special-case null vs. empty.
func (c *Client) CrashesList(ctx context.Context, sessionID string, limit int) ([]CrashEvent, error) {
	if sessionID == "" {
		return nil, errors.New("client: CrashesList requires a non-empty session_id")
	}

	q := url.Values{}
	q.Set("session", sessionID)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp crashesListRespWire
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     "/crashes?" + q.Encode(),
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return nil, err
	}

	// Defensive: never surface a nil slice — emit an empty slice so the
	// MCP tool layer can JSON-encode "[]" without the (CrashEvent)(nil)
	// → "null" pitfall.
	if resp.Crashes == nil {
		return []CrashEvent{}, nil
	}
	return resp.Crashes, nil
}
