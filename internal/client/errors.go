// Package client provides a reusable HTTP client for the tracelab hub.
//
// The client is consumed by both the CLI (Phase 2a) and the MCP server
// (Phase 2b). It speaks the same JSON API the hub exposes in
// internal/http/ and mirrors the hub's response shapes via its own DTOs
// (see types.go) — the package deliberately does NOT import the hub's
// internal/store/ or internal/http/ types, to keep Phase-1 packages
// untouched.
//
// Error mapping (see also doRequest in client.go):
//
//   - 401/403 → ErrUnauthorized (sentinel; use errors.Is)
//   - 5xx     → *HTTPError wrapping ErrServerError (sentinel)
//   - 4xx     → *HTTPError (no extra sentinel)
//   - 2xx     → no error; body decoded into the call-site struct
package client

import "errors"

// ErrUnauthorized is returned by any authenticated method when the hub
// answers 401 Unauthorized or 403 Forbidden. Use errors.Is to detect it.
var ErrUnauthorized = errors.New("client: unauthorized")

// ErrServerError is returned (wrapped inside *HTTPError) for any 5xx
// response. Callers that want to react to server-side failures
// generically — e.g. for retry — should check errors.Is(err, ErrServerError).
var ErrServerError = errors.New("client: server error")

// HTTPError is the typed error returned for non-2xx responses. It carries
// the status code, the endpoint path that produced the error, and a
// truncated copy of the response body for diagnostics.
//
// Use errors.As to extract it:
//
//	var httpErr *client.HTTPError
//	if errors.As(err, &httpErr) {
//	    log.Printf("hub said %d: %s", httpErr.Status, httpErr.Body)
//	}
type HTTPError struct {
	// Status is the HTTP status code returned by the hub.
	Status int
	// Endpoint is the request path (without query string) that failed,
	// e.g. "/session/start". Useful when logs don't preserve URLs.
	Endpoint string
	// Body is a truncated copy of the response body (max 1 KiB). May be
	// empty if the response carried no body.
	Body string
	// inner is the wrapped sentinel (ErrUnauthorized / ErrServerError /
	// nil). Exposed via Unwrap so errors.Is works without a second
	// allocation per call site.
	inner error
}

// Error implements error.
func (e *HTTPError) Error() string {
	if e.Body == "" {
		return "client: " + e.Endpoint + ": http " + itoa(e.Status)
	}
	return "client: " + e.Endpoint + ": http " + itoa(e.Status) + ": " + e.Body
}

// Unwrap returns the sentinel (if any) so errors.Is can match it.
func (e *HTTPError) Unwrap() error { return e.inner }

// itoa is a tiny zero-alloc helper avoiding a dependency on strconv in
// the hot Error() path — three-digit HTTP codes max.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
