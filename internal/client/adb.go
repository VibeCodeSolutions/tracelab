package client

import (
	"context"
	"errors"
	"net/http"
)

// ADBDevice mirrors the wire shape of one entry in GET /adb/devices.
// The hub-side type lives in internal/http/adb.go (adbDeviceView). Field
// names are JSON-stable; Model is omitted by the hub when adb did not
// report one — the empty string is the in-Go representation of "missing".
//
// State values are documented in internal/adb/adb.go (Device.State):
// "device", "offline", "unauthorized", "no permissions", "recovery",
// "sideload", "bootloader". CLI / MCP callers treat them as opaque
// strings — no enum here so the hub can evolve adb-reported states
// without a client release.
type ADBDevice struct {
	Serial string `json:"serial"`
	State  string `json:"state"`
	Model  string `json:"model,omitempty"`
}

// adbStartRespWire is the wire shape of the hub's POST /adb/start response.
// The hub-side adbStartResp type carries Status ("started" | "already_running"),
// Serial, and StartedAt; we decode it client-side but only surface success/
// error semantics to the caller — the discriminator is meaningful enough
// only when a higher-level consumer needs it (e.g. MCP server reporting
// "already running since X" to Claude Code).
type adbStartRespWire struct {
	Status    string `json:"status"`
	Serial    string `json:"serial"`
	StartedAt int64  `json:"started_at"`
}

// adbStopRespWire mirrors the hub's POST /adb/stop body. Same rationale as
// adbStartRespWire: the Status field ("stopped" | "not_running") is
// retained for future structured surfaces, but ADB Start / Stop callers
// today just need a success / error signal.
type adbStopRespWire struct {
	Status string `json:"status"`
	Serial string `json:"serial"`
}

// adbStartReqWire is the request body for POST /adb/start. Serial is
// required; Session is currently informational (the hub opens its own
// per-reconnect sessions via the bridge), but we accept an optional
// override so callers don't have to track the wire format twice.
type adbStartReqWire struct {
	Serial  string `json:"serial"`
	Session string `json:"session,omitempty"`
}

// adbStopReqWire is the request body for POST /adb/stop.
type adbStopReqWire struct {
	Serial string `json:"serial"`
}

// ListADBDevices returns the currently attached adb devices as the hub
// sees them (the hub shells out to `adb devices -l`). The slice is never
// nil — an empty list is returned when no devices are attached.
//
// Authentication: bearer-protected; 401/403 surface as ErrUnauthorized
// (wrapped in *HTTPError). 503 from the hub means adb itself is
// unreachable on the hub host — surfaced as *HTTPError without the
// ErrServerError sentinel (callers can retry on transient adb daemon
// issues without forcing the generic "server error" branch).
func (c *Client) ListADBDevices(ctx context.Context) ([]ADBDevice, error) {
	var resp []ADBDevice
	err := c.doRequest(ctx, requestOpts{
		method:   http.MethodGet,
		path:     "/adb/devices",
		auth:     true,
		respInto: &resp,
	})
	if err != nil {
		return nil, err
	}
	// Defensive: the hub always emits [] for an empty list (never null),
	// but a future intermediary or test stub might not. Normalise to a
	// non-nil empty slice so callers can range over the result without a
	// nil-check.
	if resp == nil {
		return []ADBDevice{}, nil
	}
	return resp, nil
}

// StartADBBridge asks the hub to start an adb logcat bridge for the given
// device serial. The optional sessionID is forwarded to the hub as the
// `session` field; if empty, the hub creates a fresh session on each
// bridge reconnect (the production default).
//
// Idempotency: the hub returns 200 OK whether the bridge was freshly
// started or already running (the JSON body's `status` field carries the
// discrimination — see internal/http/adb.go startHandler). StartADBBridge
// surfaces the hub's status as the first return value so callers that
// need the distinction (e.g. MCP tools reporting "already running" to
// Claude Code) can pass it through; CLI scripts that only need the
// idempotent "ensure running" semantic can ignore it via `_, err := ...`.
//
// Return values:
//   - status: "started" | "already_running" (hub-side discriminator).
//     Empty string on error.
//   - err: client-side rejection or transport error (see below).
//
// Errors:
//   - empty serial → client-side rejection (no network round-trip)
//   - 401/403 → ErrUnauthorized
//   - 5xx → ErrServerError (wrapped in *HTTPError)
//   - other 4xx → *HTTPError
func (c *Client) StartADBBridge(ctx context.Context, serial, sessionID string) (string, error) {
	if serial == "" {
		return "", errors.New("client: StartADBBridge requires a non-empty serial")
	}
	var resp adbStartRespWire
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodPost,
		path:     "/adb/start",
		body:     adbStartReqWire{Serial: serial, Session: sessionID},
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return "", err
	}
	return resp.Status, nil
}

// StopADBBridge asks the hub to stop the adb logcat bridge for the given
// device serial.
//
// Idempotency: the hub returns 200 OK whether the bridge was actively
// running or already gone (the body's `status` field is "stopped" vs.
// "not_running"). The status is surfaced as the first return value so
// callers that care about the distinction can pass it through; callers
// that only need the idempotent "ensure stopped" semantic can ignore it
// via `_, err := ...`.
//
// Errors mirror StartADBBridge (sentinel pattern via *HTTPError + Unwrap).
func (c *Client) StopADBBridge(ctx context.Context, serial string) (string, error) {
	if serial == "" {
		return "", errors.New("client: StopADBBridge requires a non-empty serial")
	}
	var resp adbStopRespWire
	if err := c.doRequest(ctx, requestOpts{
		method:   http.MethodPost,
		path:     "/adb/stop",
		body:     adbStopReqWire{Serial: serial},
		auth:     true,
		respInto: &resp,
	}); err != nil {
		return "", err
	}
	return resp.Status, nil
}
