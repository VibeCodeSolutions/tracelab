package client

import (
	"context"
	"net/http"
)

// Health probes the hub's /healthz endpoint. The endpoint is
// intentionally unauthenticated (see hub README §API and ADR in
// docs/ARCH.md) so Health does NOT attach the Authorization header —
// any 401 here would indicate misconfiguration on the hub side, not on
// the client.
//
// Returns nil on 200 OK. Non-2xx responses are surfaced via the standard
// error mapping in doRequest (5xx → *HTTPError wrapping ErrServerError).
// The 200 body is drained but not parsed; the hub returns
// {"status":"ok"} but the client treats Health as a liveness probe only.
func (c *Client) Health(ctx context.Context) error {
	return c.doRequest(ctx, requestOpts{
		method: http.MethodGet,
		path:   "/healthz",
		auth:   false,
	})
}
