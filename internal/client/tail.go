package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// tailDialTimeout caps the WebSocket handshake. Once the connection is up
// the stream is long-lived; the hub sends ping frames every 30 s so a
// silent network is not interpreted as a stall by us.
const tailDialTimeout = 10 * time.Second

// tailCloseWait bounds the time we wait for the server to acknowledge the
// graceful close frame before tearing the socket down hard.
const tailCloseWait = 1 * time.Second

// Tail subscribes to the hub's /tail WebSocket endpoint and invokes
// onEvent for every frame received. sessionFilter == "" subscribes to all
// sessions; a non-empty value is forwarded as the ?session=<id> query
// parameter (the hub-side ws.Handler reads it).
//
// Lifecycle:
//
//   - ctx cancellation initiates a graceful close: a CloseNormal (1000)
//     frame is sent, the underlying connection is shut down, and Tail
//     returns nil (treating user-driven cancel as success).
//   - A server-side close (CloseGoingAway, hub shutdown) also returns nil.
//   - Any other read error — network drop, handshake failure, JSON decode
//     failure — is returned wrapped for diagnostics.
//
// Auth contract:
//
//   - 401 / 403 at handshake → *HTTPError wrapping ErrUnauthorized
//     (same sentinel the HTTP methods return; callers can branch on
//     errors.Is(err, ErrUnauthorized)).
//   - 5xx at handshake → *HTTPError wrapping ErrServerError.
//   - other non-1xx → *HTTPError.
//
// No reconnect logic: a dropped stream returns; callers decide if/how to
// retry. ADR-003 explicitly defers reconnect to a later sprint.
func (c *Client) Tail(ctx context.Context, sessionFilter string, onEvent func(Event)) error {
	if onEvent == nil {
		return errors.New("client: Tail requires a non-nil onEvent callback")
	}

	dialURL, err := c.tailURL(sessionFilter)
	if err != nil {
		return err
	}

	dialer := *websocket.DefaultDialer // copy so we don't mutate the package global
	dialer.HandshakeTimeout = tailDialTimeout

	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.token)

	conn, resp, err := dialer.DialContext(ctx, dialURL, header)
	if err != nil {
		return c.tailDialError(resp, err)
	}
	defer conn.Close()

	// Watcher goroutine: when the caller's context fires, send a clean
	// close frame and shut the connection down. That unblocks ReadJSON
	// below with a CloseError, which we translate to a nil return.
	//
	// Owner pattern: this goroutine is owned by Tail; doneCh is closed
	// when Tail returns so the watcher can exit even when ctx never fires
	// (e.g. server-initiated close path).
	doneCh := make(chan struct{})
	defer close(doneCh)
	go func() {
		select {
		case <-ctx.Done():
			// Best-effort graceful close. We ignore errors — if the
			// connection is already torn down the writes simply fail and
			// the main loop will exit anyway.
			deadline := time.Now().Add(tailCloseWait)
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client cancel"),
				deadline,
			)
			_ = conn.Close()
		case <-doneCh:
			// Tail is returning for another reason; nothing to do.
		}
	}()

	for {
		var evt Event
		if err := conn.ReadJSON(&evt); err != nil {
			return c.tailReadError(ctx, err)
		}
		onEvent(evt)
	}
}

// tailURL builds the ws:// (or wss://) URL for the /tail endpoint. The
// scheme is derived from baseURL — http → ws, https → wss — so callers
// don't have to maintain two URLs.
func (c *Client) tailURL(sessionFilter string) (string, error) {
	u := *c.baseURL // copy; baseURL must stay immutable for concurrent callers
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("client: Tail: unsupported scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/tail"
	if sessionFilter != "" {
		q := url.Values{}
		q.Set("session", sessionFilter)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// tailDialError maps a websocket.Dialer error into our *HTTPError /
// sentinel shape so callers can use the same errors.Is pattern they use
// for the HTTP methods.
//
// When the handshake reaches the server far enough to produce an HTTP
// response (the common case for 401/403/5xx), resp carries the status
// code and we attach the matching sentinel.
func (c *Client) tailDialError(resp *http.Response, err error) error {
	if resp != nil {
		httpErr := &HTTPError{
			Status:   resp.StatusCode,
			Endpoint: "/tail",
		}
		// Best-effort body snippet for diagnostics (same shape as doRequest).
		if resp.Body != nil {
			httpErr.Body = readSnippet(resp.Body)
			_ = resp.Body.Close()
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			httpErr.inner = ErrUnauthorized
		case resp.StatusCode >= 500:
			httpErr.inner = ErrServerError
		}
		return httpErr
	}
	return fmt.Errorf("client: GET /tail: %w", err)
}

// tailReadError translates a ReadJSON error into the Tail return value.
//
// User-driven cancellation (ctx fired, then our watcher closed the conn)
// surfaces as a CloseError / net-closed error — we report nil since the
// caller explicitly asked for the stream to stop. The same applies to a
// server-initiated CloseGoingAway (hub shutdown): graceful termination.
//
// Any other error — unexpected close codes, network drops mid-frame, JSON
// decode failures — is wrapped and returned for the caller to log.
func (c *Client) tailReadError(ctx context.Context, err error) error {
	// Caller cancelled — graceful exit regardless of the underlying error.
	if ctx.Err() != nil {
		return nil
	}
	// Clean server close — normal end of stream.
	if websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
	) {
		return nil
	}
	return fmt.Errorf("client: /tail read: %w", err)
}
