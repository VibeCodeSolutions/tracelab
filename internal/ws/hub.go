// Package ws implements the WebSocket pub/sub layer for tracelab's live
// event tail (`GET /tail`).
//
// The Hub is the central fan-out: HTTP /ingest pushes events into the Hub,
// the Hub broadcasts them to all matching subscribers via per-subscriber
// buffered channels. Slow subscribers are dropped (channel-full) rather
// than blocking the publisher — observability beats lossless delivery here.
//
// Concurrency model:
//   - Hub state (subscribers map) is guarded by a sync.RWMutex.
//   - Publish takes the read lock and does non-blocking sends; subscribers
//     never block the ingest path.
//   - Subscribe / Unsubscribe take the write lock briefly.
//   - Close terminates all subscriber channels, signalling write-pumps to
//     exit and flush a websocket close frame.
package ws

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// Event is the wire representation of a single tracelab event broadcast
// to /tail subscribers. Fields mirror handlers.ingestEvent + the session id
// so a single client can demultiplex multiple sessions.
type Event struct {
	SessionID string          `json:"session_id"`
	TS        int64           `json:"ts"`
	Source    string          `json:"source"`
	Level     string          `json:"level"`
	Msg       string          `json:"msg"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

// subscriber is a single /tail client's mailbox. SessionFilter == "" means
// "match all sessions". The channel is buffered; on overflow the publisher
// drops the event for this subscriber rather than blocking.
type subscriber struct {
	id            uint64
	sessionFilter string
	ch            chan Event
}

// Hub is the pub/sub dispatcher shared between the /ingest handler (publisher)
// and /tail handlers (subscribers).
//
// The zero value is not usable; obtain one via NewHub.
type Hub struct {
	mu       sync.RWMutex
	subs     map[uint64]*subscriber
	nextID   atomic.Uint64
	bufSize  int
	closed   bool
	closedCh chan struct{}
}

// NewHub returns a ready-to-use Hub. bufSize sets the per-subscriber channel
// buffer; values <=0 default to 64.
func NewHub(bufSize int) *Hub {
	if bufSize <= 0 {
		bufSize = 64
	}
	return &Hub{
		subs:     make(map[uint64]*subscriber),
		bufSize:  bufSize,
		closedCh: make(chan struct{}),
	}
}

// Subscribe registers a new subscriber and returns its event channel along
// with an unsubscribe func. sessionFilter == "" subscribes to all sessions;
// otherwise only events matching that session id are delivered.
//
// The unsubscribe func is idempotent and safe to call from any goroutine.
// After Hub.Close, Subscribe returns a closed channel and a no-op cancel.
func (h *Hub) Subscribe(sessionFilter string) (<-chan Event, func()) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		ch := make(chan Event)
		close(ch)
		return ch, func() {}
	}
	id := h.nextID.Add(1)
	s := &subscriber{
		id:            id,
		sessionFilter: sessionFilter,
		ch:            make(chan Event, h.bufSize),
	}
	h.subs[id] = s
	h.mu.Unlock()

	cancel := func() { h.unsubscribe(id) }
	return s.ch, cancel
}

// unsubscribe removes the subscriber and closes its channel exactly once.
// Calling it twice is harmless; calling it after Close is harmless.
func (h *Hub) unsubscribe(id uint64) {
	h.mu.Lock()
	s, ok := h.subs[id]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.subs, id)
	h.mu.Unlock()
	close(s.ch)
}

// Publish broadcasts evt to every matching subscriber via a non-blocking
// send. If a subscriber's channel is full, the event is dropped for that
// subscriber and the publisher continues — slow consumers must not stall
// the ingest path.
//
// Publish on a closed Hub is a no-op.
func (h *Hub) Publish(evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return
	}
	for _, s := range h.subs {
		if s.sessionFilter != "" && s.sessionFilter != evt.SessionID {
			continue
		}
		select {
		case s.ch <- evt:
		default:
			// drop on full — slow subscriber, not the publisher's problem
		}
	}
}

// SubscriberCount returns the current number of active subscribers.
// Intended for tests and /healthz-style introspection.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// Done returns a channel that is closed when Hub.Close has been called.
// /tail handlers select on this to know when the server is shutting down
// and they should send a close frame to their client.
func (h *Hub) Done() <-chan struct{} { return h.closedCh }

// Close marks the hub as shut down, closes all subscriber channels and
// signals Done(). Idempotent.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	subs := h.subs
	h.subs = make(map[uint64]*subscriber)
	h.mu.Unlock()

	for _, s := range subs {
		close(s.ch)
	}
	close(h.closedCh)
}
