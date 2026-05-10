package ws

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Heartbeat / timeout knobs. Exposed as package-level vars so tests can
// shrink them without changing the production defaults.
var (
	// PongWait is the maximum time a client has to respond to a ping frame
	// before its connection is considered dead.
	PongWait = 60 * time.Second
	// PingPeriod must be < PongWait. A ping is sent every PingPeriod.
	PingPeriod = 30 * time.Second
	// WriteWait bounds a single write (event or ping) before we give up.
	WriteWait = 10 * time.Second
)

// upgrader is shared across requests. CheckOrigin is permissive because the
// /tail endpoint is already protected by Bearer auth at the chi-middleware
// layer; browsers calling cross-origin would still need to forward the
// Authorization header explicitly.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// Handler returns an http.HandlerFunc that upgrades incoming requests to
// WebSocket and streams matching events from the hub until either the
// client drops, the heartbeat times out or the hub is closed.
//
// Query param `session=<id>` restricts the stream to one session;
// without it the client receives events for all sessions.
//
// logger may be nil — slog.Default() is used in that case.
func Handler(h *Hub, logger *slog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		sessionFilter := r.URL.Query().Get("session")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrade already wrote an error response.
			logger.Warn("ws upgrade failed", slog.Any("error", err))
			return
		}

		events, cancel := h.Subscribe(sessionFilter)
		// servePump is responsible for closing the conn and calling cancel
		// exactly once; do not defer either here.
		servePump(r.Context(), conn, h, events, cancel, sessionFilter, logger)
	}
}

// servePump runs the per-connection read+write pumps. It returns once the
// connection is fully drained and closed.
//
// Owner-pattern: this goroutine owns conn.Close and cancel; the read pump
// runs as a child goroutine and signals exit via the readDone channel.
func servePump(
	parentCtx context.Context,
	conn *websocket.Conn,
	hub *Hub,
	events <-chan Event,
	cancel func(),
	sessionFilter string,
	logger *slog.Logger,
) {
	// Single defer chain — cancel and close both run exactly once.
	defer func() {
		cancel()
		_ = conn.Close()
	}()

	// Read deadline / pong handler: client must respond to our pings.
	_ = conn.SetReadDeadline(time.Now().Add(PongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(PongWait))
	})

	readDone := make(chan struct{})
	// Read pump: we expect no client messages on /tail, but we still need
	// to call ReadMessage to drive control-frame handling (pong, close)
	// and to detect client disconnects.
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	pingTicker := time.NewTicker(PingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case evt, ok := <-events:
			if !ok {
				// Hub closed (or unsubscribed externally). Send close frame
				// and exit.
				_ = conn.SetWriteDeadline(time.Now().Add(WriteWait))
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"))
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := conn.WriteJSON(evt); err != nil {
				logger.Debug("ws write failed", slog.Any("error", err), slog.String("session", sessionFilter))
				return
			}

		case <-pingTicker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logger.Debug("ws ping failed", slog.Any("error", err))
				return
			}

		case <-hub.Done():
			_ = conn.SetWriteDeadline(time.Now().Add(WriteWait))
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"))
			return

		case <-parentCtx.Done():
			_ = conn.SetWriteDeadline(time.Now().Add(WriteWait))
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "request cancelled"))
			return

		case <-readDone:
			// Client closed the connection (or pong timeout).
			return
		}
	}
}
