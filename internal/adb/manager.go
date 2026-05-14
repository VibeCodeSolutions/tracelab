// BridgeManager owns the per-serial lifecycle for hub-managed adb bridges.
// It is the runtime surface behind the hub's POST /adb/start, POST /adb/stop
// and GET /adb/devices endpoints (see internal/http/adb.go).
//
// Design contract:
//
//   - Each serial maps to at most one running Bridge. Start on an already
//     running serial is idempotent and reports ErrBridgeAlreadyRunning.
//   - Stop on an unknown serial is idempotent and reports ErrBridgeNotRunning.
//   - All state transitions are serialised through the manager mutex, so
//     concurrent Start/Stop calls on the same serial can never corrupt the
//     map or leak goroutines.
//   - Bridge goroutines are owned by the manager: Stop cancels the per-bridge
//     context and waits for Run to return before clearing the map entry, so
//     a follow-up Start on the same serial never races a still-shutting-down
//     bridge.
//   - Close tears down every running bridge in parallel and is safe to call
//     more than once.
//
// Construction:
//
//   - NewBridgeManager(deps) captures the shared store + ws-hub + factory
//     once; per-start callers only supply BridgeStartOptions for the
//     per-invocation knobs (serial, tag filter, session label override).
//
// Manager is intentionally decoupled from internal/http/ — the HTTP layer
// translates manager errors into status codes (start-already-running →
// 200/409 per implementer decision; stop-not-running → 200/404).
package adb

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrBridgeAlreadyRunning is returned by BridgeManager.Start when a bridge
// for the given serial is already active. Use errors.Is to detect it.
var ErrBridgeAlreadyRunning = errors.New("adb: bridge already running")

// ErrBridgeNotRunning is returned by BridgeManager.Stop when no bridge is
// active for the given serial. Use errors.Is to detect it.
var ErrBridgeNotRunning = errors.New("adb: bridge not running")

// BridgeFactory constructs a Bridge from a config. Production code uses
// NewBridge; tests inject a stub factory to avoid spawning real adb
// subprocesses or SQLite-backed stores.
type BridgeFactory func(BridgeConfig) *Bridge

// BridgeManagerDeps bundles the shared dependencies every managed bridge
// uses: the store, the optional ws hub, the logger, and the factory.
type BridgeManagerDeps struct {
	// Store is the persistence backend handed to every bridge. Required.
	Store BridgeStore
	// Hub is the optional websocket fan-out. nil disables live broadcast
	// for managed bridges (persistence still runs).
	Hub BridgePublisher
	// Logger defaults to slog.Default() when nil.
	Logger *slog.Logger
	// Factory defaults to NewBridge when nil. Tests override this.
	Factory BridgeFactory
}

// BridgeStartOptions carries the per-invocation knobs for Start. All
// fields are optional except DeviceSerial.
type BridgeStartOptions struct {
	// DeviceSerial selects the adb device. Required and used as the map
	// key inside the manager.
	DeviceSerial string
	// TagFilter restricts logcat to a single tag. Empty means all tags.
	TagFilter string
	// BatchInterval / BatchSize / BackoffSchedule mirror BridgeConfig and
	// are passed through verbatim. Zero values fall back to BridgeConfig
	// defaults.
	BatchInterval   time.Duration
	BatchSize       int
	BackoffSchedule []time.Duration
}

// BridgeStatus is the lightweight snapshot returned by Start / Status.
// Sessions are tracked indirectly via the store; the manager only knows
// "is a bridge running for this serial, and when did it start".
type BridgeStatus struct {
	// DeviceSerial echoes the start request.
	DeviceSerial string
	// StartedAt is the unix-nanosecond timestamp when Start succeeded.
	// Useful for the HTTP layer's "already_running since X" response.
	StartedAt int64
}

// runningBridge is the per-serial bookkeeping kept inside the manager.
type runningBridge struct {
	status BridgeStatus
	cancel context.CancelFunc
	// done closes when the Run goroutine returns. Stop blocks on it so
	// a subsequent Start on the same serial cannot race a teardown.
	done chan struct{}
}

// BridgeManager is the goroutine-safe coordinator for hub-managed adb
// bridges. Construct via NewBridgeManager.
type BridgeManager struct {
	deps BridgeManagerDeps

	mu      sync.Mutex
	bridges map[string]*runningBridge
	closed  bool
}

// NewBridgeManager returns a ready-to-use manager. Returns nil when
// deps.Store is unset — the manager has no fallback for missing persistence,
// matching the Bridge contract.
func NewBridgeManager(deps BridgeManagerDeps) *BridgeManager {
	if deps.Store == nil {
		return nil
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Factory == nil {
		deps.Factory = NewBridge
	}
	return &BridgeManager{
		deps:    deps,
		bridges: make(map[string]*runningBridge),
	}
}

// Start brings up a bridge for opts.DeviceSerial. On success it returns the
// new BridgeStatus. If a bridge is already running for that serial the
// pre-existing status is returned together with ErrBridgeAlreadyRunning —
// callers (the HTTP layer) decide whether to surface 200 OK or 409 Conflict.
//
// Empty DeviceSerial is rejected as a programmer error: the manager keys
// bridges by serial, and an "<auto>" entry would collide with the next
// start. Hub callers MUST supply a concrete serial.
//
// Start spawns the Bridge's Run goroutine under a context derived from
// context.Background() — managed bridges live for the lifetime of the
// manager, not of the HTTP request that started them.
func (m *BridgeManager) Start(opts BridgeStartOptions) (BridgeStatus, error) {
	if opts.DeviceSerial == "" {
		return BridgeStatus{}, errors.New("adb: BridgeManager.Start requires a non-empty DeviceSerial")
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return BridgeStatus{}, errors.New("adb: bridge manager is closed")
	}
	if existing, ok := m.bridges[opts.DeviceSerial]; ok {
		status := existing.status
		m.mu.Unlock()
		return status, ErrBridgeAlreadyRunning
	}

	cfg := BridgeConfig{
		DeviceSerial:    opts.DeviceSerial,
		TagFilter:       opts.TagFilter,
		Store:           m.deps.Store,
		Hub:             m.deps.Hub,
		Logger:          m.deps.Logger,
		BatchInterval:   opts.BatchInterval,
		BatchSize:       opts.BatchSize,
		BackoffSchedule: opts.BackoffSchedule,
	}
	br := m.deps.Factory(cfg)
	if br == nil {
		m.mu.Unlock()
		return BridgeStatus{}, errors.New("adb: bridge factory returned nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	rb := &runningBridge{
		status: BridgeStatus{
			DeviceSerial: opts.DeviceSerial,
			StartedAt:    time.Now().UnixNano(),
		},
		cancel: cancel,
		done:   make(chan struct{}),
	}
	m.bridges[opts.DeviceSerial] = rb
	// Snapshot status while we still hold the lock; the goroutine will
	// outlive this critical section.
	status := rb.status
	m.mu.Unlock()

	go func() {
		defer close(rb.done)
		if err := br.Run(ctx); err != nil {
			m.deps.Logger.Error("adb managed bridge exited",
				slog.String("device_serial", opts.DeviceSerial),
				slog.Any("error", err),
			)
		}
		// Run returned for some other reason than Stop (e.g. unrecoverable
		// startup error or, in theory, future Run-side termination paths).
		// Make sure the map entry is cleared so a future Start can succeed.
		m.mu.Lock()
		if cur, ok := m.bridges[opts.DeviceSerial]; ok && cur == rb {
			delete(m.bridges, opts.DeviceSerial)
		}
		m.mu.Unlock()
	}()

	return status, nil
}

// Stop tears down the bridge for serial. Returns ErrBridgeNotRunning when
// no bridge is registered for that serial. Otherwise it cancels the bridge
// context, waits for Run to return (so the caller can rely on the resource
// having been released), and removes the map entry.
//
// Concurrent Stop calls on the same serial: the first call wins, the
// second sees ErrBridgeNotRunning. This is the contract the HTTP layer
// surfaces to callers (idempotent stop).
func (m *BridgeManager) Stop(serial string) error {
	m.mu.Lock()
	rb, ok := m.bridges[serial]
	if !ok {
		m.mu.Unlock()
		return ErrBridgeNotRunning
	}
	// Remove from the map up-front so a concurrent Start can proceed once
	// we release the lock; the done-channel wait below still guarantees we
	// don't return before the goroutine has actually exited.
	delete(m.bridges, serial)
	m.mu.Unlock()

	rb.cancel()
	<-rb.done
	return nil
}

// Status returns a snapshot of the running bridges. The slice is safe for
// the caller to mutate; the map ordering is unspecified — the HTTP layer
// sorts on its own if it needs stability for clients.
func (m *BridgeManager) Status() []BridgeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]BridgeStatus, 0, len(m.bridges))
	for _, rb := range m.bridges {
		out = append(out, rb.status)
	}
	return out
}

// IsRunning reports whether a bridge is currently registered for serial.
// Useful for tests; production callers go through Start/Stop directly.
func (m *BridgeManager) IsRunning(serial string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.bridges[serial]
	return ok
}

// Close stops every running bridge and prevents further Start calls. Safe
// to call more than once; subsequent calls are no-ops.
func (m *BridgeManager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	bridges := m.bridges
	m.bridges = map[string]*runningBridge{}
	m.mu.Unlock()

	// Cancel every bridge in parallel; wait on each done channel so Close
	// only returns once all goroutines are actually released.
	for _, rb := range bridges {
		rb.cancel()
	}
	for _, rb := range bridges {
		<-rb.done
	}
}
