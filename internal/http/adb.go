package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/adb"
)

// devicesProbeTimeout caps how long GET /adb/devices is allowed to spend
// shelling out to `adb devices -l`. Short enough so a hung adb daemon
// surfaces as a 503, long enough to cover the cold-start "* daemon not
// running; starting" path on a typical dev box.
const devicesProbeTimeout = 5 * time.Second

// adbDeviceLister is the minimal interface the hub needs to enumerate
// attached devices. Tests inject a stub; production code uses adb.Devices
// directly via the adapter in server.go.
type adbDeviceLister interface {
	Devices(ctx context.Context) ([]adb.Device, error)
}

// adbBridgeManager is the subset of *adb.BridgeManager the HTTP handlers
// touch. Defined here so server_test.go can substitute a fake without a
// live store / hub.
type adbBridgeManager interface {
	Start(opts adb.BridgeStartOptions) (adb.BridgeStatus, error)
	Stop(serial string) error
}

// adbHandlers groups the per-route adb handler funcs with their
// dependencies. They are wired into the main router by server.New when
// both the device lister and bridge manager are present.
type adbHandlers struct {
	lister  adbDeviceLister
	manager adbBridgeManager
}

// adbDeviceView is the wire shape for GET /adb/devices. It is a deliberate
// subset of adb.Device — Model is the only optional field, omitempty when
// adb did not report one. The CLI table renders SERIAL/STATE/MODEL, which
// is exactly this view's columns.
type adbDeviceView struct {
	Serial string `json:"serial"`
	State  string `json:"state"`
	Model  string `json:"model,omitempty"`
}

// adbDevicesHandler implements GET /adb/devices. Returns a JSON array of
// adbDeviceView (never null — an empty slice is encoded as []).
//
// Errors from the lister surface as 503 Service Unavailable: adb itself
// being unreachable is a service-condition, not a request-validation
// problem, and clients should retry rather than treat it as a permanent
// failure.
func (h *adbHandlers) devicesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), devicesProbeTimeout)
	defer cancel()

	devices, err := h.lister.Devices(ctx)
	if err != nil {
		// Treat adb unavailability as 503 — the hub itself is up, but it
		// cannot satisfy this endpoint right now.
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "adb unavailable: " + err.Error(),
		})
		return
	}

	out := make([]adbDeviceView, 0, len(devices))
	for _, d := range devices {
		out = append(out, adbDeviceView{
			Serial: d.Serial,
			State:  d.State,
			Model:  d.Model,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// adbStartReq is the body for POST /adb/start. Serial is required; Session
// is currently informational only — the bridge always opens its own per-
// reconnect session via the store. The field is accepted for forward-
// compatibility (a future bridge mode could attach to an existing session).
type adbStartReq struct {
	Serial  string `json:"serial"`
	Session string `json:"session,omitempty"`
}

// adbStartResp is the success body for POST /adb/start.
//
// Status values:
//
//   - "started":         a fresh bridge was launched. StartedAt is the
//     unix-nanosecond timestamp.
//   - "already_running": the bridge was already active. StartedAt echoes
//     when it originally started so callers can detect long-running
//     bridges. Returned together with HTTP 200 (idempotent) — see
//     internal/http/adb.go startHandler for the rationale.
type adbStartResp struct {
	Status    string `json:"status"`
	Serial    string `json:"serial"`
	StartedAt int64  `json:"started_at"`
}

// adbStartHandler implements POST /adb/start.
//
// Idempotency contract (implementer decision, recorded in WORKLOG #015):
//
//   - Fresh start succeeds with 200 OK + {"status":"started", ...}.
//   - Start on an already running bridge ALSO returns 200 OK, but with
//     {"status":"already_running", ...} and the original StartedAt. The
//     200-with-discriminator-body shape was chosen over 409 Conflict so
//     CLI/MCP scripts can `tracelab adb start <serial>` in idempotent
//     "ensure-running" pipelines without branching on HTTP status —
//     the JSON status field carries the disambiguation. This matches the
//     /ingest pattern (always 202, body says how many lines accepted).
//
// Errors:
//
//   - 400 if the body is invalid or serial is empty.
//   - 500 if BridgeManager.Start returns a non-idempotency error.
func (h *adbHandlers) startHandler(w http.ResponseWriter, r *http.Request) {
	var req adbStartReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Serial == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "serial required"})
		return
	}

	status, err := h.manager.Start(adb.BridgeStartOptions{DeviceSerial: req.Serial})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, adbStartResp{
			Status:    "started",
			Serial:    status.DeviceSerial,
			StartedAt: status.StartedAt,
		})
	case errors.Is(err, adb.ErrBridgeAlreadyRunning):
		writeJSON(w, http.StatusOK, adbStartResp{
			Status:    "already_running",
			Serial:    status.DeviceSerial,
			StartedAt: status.StartedAt,
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "adb start failed: " + err.Error(),
		})
	}
}

// adbStopReq is the body for POST /adb/stop. Serial is required.
type adbStopReq struct {
	Serial string `json:"serial"`
}

// adbStopResp is the success body for POST /adb/stop.
//
// Status values:
//
//   - "stopped":     a running bridge was torn down.
//   - "not_running": no bridge was registered for the serial. The 200-with-
//     discriminator pattern mirrors startHandler — idempotent stop scripts
//     do not need to special-case "already stopped".
type adbStopResp struct {
	Status string `json:"status"`
	Serial string `json:"serial"`
}

// adbStopHandler implements POST /adb/stop. Idempotent: stop on a serial
// that is not currently running returns 200 OK with status "not_running"
// rather than 404. Rationale matches startHandler (CLI/MCP "ensure-stopped"
// pipelines should not have to branch on HTTP status).
func (h *adbHandlers) stopHandler(w http.ResponseWriter, r *http.Request) {
	var req adbStopReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Serial == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "serial required"})
		return
	}

	err := h.manager.Stop(req.Serial)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, adbStopResp{
			Status: "stopped",
			Serial: req.Serial,
		})
	case errors.Is(err, adb.ErrBridgeNotRunning):
		writeJSON(w, http.StatusOK, adbStopResp{
			Status: "not_running",
			Serial: req.Serial,
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "adb stop failed: " + err.Error(),
		})
	}
}

