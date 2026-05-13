package client

import (
	"context"
	"errors"
	"net/http"
	"strconv"
)

// StartSession opens a new test/debug session on the hub. Label is
// free-form and may be empty; the hub records it verbatim. Returns the
// hub-assigned session ID on success.
func (c *Client) StartSession(ctx context.Context, label string) (string, error) {
	var resp sessionStartRespWire
	err := c.doRequest(ctx, requestOpts{
		method:   http.MethodPost,
		path:     "/session/start",
		body:     sessionStartReqWire{Label: label},
		auth:     true,
		respInto: &resp,
	})
	if err != nil {
		return "", err
	}
	return resp.SessionID, nil
}

// EndSession marks a session as ended on the hub. Returns nil on the
// hub's 204 No Content. A hub-side 404 (unknown session) is surfaced as
// a *HTTPError with Status == 404 — callers that want to ignore "already
// ended / never existed" should check `errors.As` + Status.
func (c *Client) EndSession(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("client: EndSession requires a non-empty id")
	}
	return c.doRequest(ctx, requestOpts{
		method: http.MethodPost,
		path:   "/session/end",
		body:   sessionEndReqWire{SessionID: id},
		auth:   true,
	})
}

// ListSessions fetches the most recent sessions from the hub. When limit
// is <= 0, the hub's server-side default (50) is used. The hub caps the
// limit at 1000 silently — values above are accepted but quietly
// clamped, mirroring the hub behaviour.
func (c *Client) ListSessions(ctx context.Context, limit int) ([]Session, error) {
	path := "/sessions"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var resp listSessionsRespWire
	err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     path,
		auth:     true,
		respInto: &resp,
	})
	if err != nil {
		return nil, err
	}
	// Defensive: never return a nil slice — callers can range cleanly.
	if resp.Sessions == nil {
		return []Session{}, nil
	}
	return resp.Sessions, nil
}
