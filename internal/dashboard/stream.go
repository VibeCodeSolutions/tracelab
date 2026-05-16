// stream.go — SSE live-tail handler for the dashboard (Phase 2c S2,
// ADR-012 Accepted).
//
// GET /dashboard/stream?session=<id> opens a Server-Sent Events stream
// that bridges the existing internal/ws.Hub pub/sub fan-out to the
// browser. The handler subscribes to the Hub with the same session
// filter the WS /tail endpoint uses, encodes each event as a `data:
// <json>\n\n` SSE frame, and emits a `: heartbeat\n\n` comment line
// every HeartbeatInterval to keep proxies and net.Conn read deadlines
// from closing an idle stream.
//
// Lifecycle / shutdown contract:
//   - The handler returns when (a) the client disconnects
//     (r.Context().Done()), (b) the Hub itself shuts down (Hub.Done()
//     close), or (c) the Hub-subscriber channel is closed externally
//     (Hub.Close() while we were ranging). In all three cases the
//     deferred Hub-cancel runs, the goroutine exits, and the
//     ResponseWriter is left in a state http.Server can finalise.
//   - Slow-subscriber backpressure is inherited from the Hub
//     publisher: ws.Hub.Publish drops events on a full subscriber
//     channel rather than blocking the ingest path (ADR-012
//     Consequences). The SSE bridge therefore does not need its own
//     drop policy — if the Hub drops, this handler simply never sees
//     the event, and the browser experiences the same "occasional
//     gap under sustained backpressure" contract as a /tail WS
//     consumer.
//
// Auth posture: registered outside the bearer-group (ADR-011
// Consequences, permanently Loopback-only). Single-user dev host.
package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// HeartbeatInterval is the cadence at which `: heartbeat\n\n` SSE
// comment frames are written. 15 s is well under the conventional 30 s
// reverse-proxy idle-timeout floor (we don't ship one today but the
// margin lets us add one later without changing the protocol) and
// well under the browser's own ~90 s EventSource keep-alive. Exposed
// as a package-level var so tests can shrink it without changing the
// production default.
var HeartbeatInterval = 15 * time.Second

// StreamHandler returns the http.HandlerFunc for the SSE live-tail
// endpoint. hub must be non-nil; if it is, the constructor in
// internal/http omits the route entirely (analogous to the
// existing /tail-WS posture).
//
// Wire shape:
//
//	GET /dashboard/stream?session=<id>
//	→ 200 OK
//	   Content-Type: text/event-stream
//	   Cache-Control: no-cache
//	   Connection: keep-alive
//	   X-Accel-Buffering: no
//	   <frames>
//
// Each event becomes a single `data: <json>\n\n` frame; the JSON
// payload is the wire shape of ws.Event (same field names as the
// /tail WS endpoint). Heartbeats are SSE comment lines
// (`: heartbeat\n\n`) and are ignored by the EventSource consumer.
//
// Error shapes (all written before any frames go out):
//   - 400 Bad Request: `session` query param missing or empty
//   - 500 Internal Server Error: ResponseWriter does not implement
//     http.Flusher (no stream possible)
func (h *Handler) StreamHandler(hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionFilter := r.URL.Query().Get("session")
		if sessionFilter == "" {
			http.Error(w, "session required", http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			// httptest and net/http both implement Flusher on
			// the default ResponseWriter; this branch exists for
			// middleware-wrapped writers that hide the interface.
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// SSE response headers — set before any body bytes go out so
		// proxies see them on the initial flush.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// X-Accel-Buffering: no disables nginx response buffering for
		// SSE; harmless when nginx isn't in the path. Documented
		// upstream as the canonical SSE-friendly toggle.
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		events, cancel := hub.Subscribe(sessionFilter)
		defer cancel()

		ticker := time.NewTicker(HeartbeatInterval)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case evt, open := <-events:
				if !open {
					// Hub closed the channel (Hub.Close or
					// external unsubscribe). End the stream
					// cleanly — the browser's EventSource
					// will auto-reconnect.
					return
				}
				if err := writeSSEEvent(w, evt); err != nil {
					// Client disconnected mid-write or the
					// underlying conn errored. Returning
					// runs the deferred cancel.
					h.log.Debug("dashboard stream: write failed",
						"session", sessionFilter, "error", err)
					return
				}
				flusher.Flush()

			case <-ticker.C:
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					h.log.Debug("dashboard stream: heartbeat failed",
						"session", sessionFilter, "error", err)
					return
				}
				flusher.Flush()

			case <-hub.Done():
				// Hub-wide shutdown (the daemon is going
				// down). Exit immediately.
				return

			case <-ctx.Done():
				// Client disconnected. Hub-cancel runs via
				// the deferred call above.
				return
			}
		}
	}
}

// writeSSEEvent encodes one ws.Event as a single SSE `data:` frame.
// The JSON payload is produced by encoding/json directly into a
// bytes.Buffer-equivalent flow — no intermediate buffer for a single
// small event payload.
//
// SSE frame shape:
//
//	data: <json>\n\n
//
// The trailing `\n\n` is the SSE event terminator; the leading
// `data: ` prefix marks the line as the event payload (vs. `event:`,
// `id:`, or `retry:` which are control fields we don't use).
func writeSSEEvent(w http.ResponseWriter, evt ws.Event) error {
	payload, err := json.Marshal(evt)
	if err != nil {
		// Defensive: ws.Event is a plain struct of marshalable
		// fields, so this should never fire. If it does, dropping
		// the event silently is the right call — the alternative
		// is closing the stream over a single malformed event.
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return nil
}
