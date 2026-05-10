// Bridge wires LogcatStream into the tracelab hub's event store and
// websocket fan-out. It is the daemon-facing counterpart to the
// library functions Devices() and LogcatStream(): the library produces
// raw lines, the bridge persists+broadcasts them, owns the per-reconnect
// session and survives subprocess exits via exponential backoff.
//
// Lifecycle: caller owns the parent context, calls Run(ctx) in its own
// goroutine, and signals shutdown by cancelling that context. Run
// returns nil after the inner LogcatStream stops, all pending events
// are flushed and the active session is closed. It only returns an
// error for unrecoverable startup problems (e.g. CreateSession failing
// before any logcat subprocess has been started); subprocess-level
// failures trigger a reconnect.
package adb

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// BridgeStore is the subset of *store.Store the bridge needs. Defined as
// an interface so cmd/hub tests can inject a fake without standing up a
// real SQLite database.
type BridgeStore interface {
	CreateSession(ctx context.Context, label string) (string, error)
	InsertEvents(ctx context.Context, sessionID string, events []store.Event) error
	EndSession(ctx context.Context, id string) error
}

// BridgePublisher is the subset of *ws.Hub the bridge uses for fan-out.
// Same rationale as BridgeStore — keeps the bridge testable in isolation.
type BridgePublisher interface {
	Publish(evt ws.Event)
}

// streamFunc is the function used to start a logcat subprocess. The
// production value is LogcatStream; tests inject their own to feed
// canned lines without spawning a real adb binary.
type streamFunc func(ctx context.Context, deviceSerial, tagFilter string) (<-chan LogcatLine, error)

// BridgeConfig parameterises a Bridge. All fields are optional except
// Store; sensible defaults are filled in by NewBridge.
type BridgeConfig struct {
	// DeviceSerial selects the adb device. Empty means "let adb pick".
	DeviceSerial string
	// TagFilter restricts logcat to a single tag. Empty means all tags.
	TagFilter string
	// Store is the persistence backend. Required.
	Store BridgeStore
	// Hub is the websocket fan-out. Optional: a nil hub disables
	// live broadcast but persistence still runs.
	Hub BridgePublisher
	// Logger is the structured logger. Defaults to slog.Default().
	Logger *slog.Logger

	// BatchInterval is the maximum time the bridge buffers lines before
	// flushing to the store. Default 50ms.
	BatchInterval time.Duration
	// BatchSize is the maximum number of lines per batch insert. The
	// bridge flushes whichever of (BatchInterval, BatchSize) trips
	// first. Default 50.
	BatchSize int

	// BackoffSchedule is the per-reconnect-attempt wait sequence. Once
	// the slice is exhausted the last value is reused indefinitely.
	// Default: 1s, 2s, 5s, 10s (then 10s forever).
	BackoffSchedule []time.Duration

	// stream is the LogcatStream override for tests. Production code
	// leaves this nil and the bridge uses the package-level function.
	stream streamFunc
}

// Bridge is the daemon-side adb logcat ingest goroutine. Construct via
// NewBridge, start via Run, stop by cancelling the context passed to Run.
type Bridge struct {
	cfg BridgeConfig
}

// defaultBackoff is the production reconnect schedule. Kept package-level
// so tests can reference it for parity assertions if needed.
var defaultBackoff = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
}

// NewBridge applies defaults and returns a runnable Bridge. Returns
// nil if Store is unset — the bridge has no fallback for missing
// persistence.
func NewBridge(cfg BridgeConfig) *Bridge {
	if cfg.Store == nil {
		return nil
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.BatchInterval <= 0 {
		cfg.BatchInterval = 50 * time.Millisecond
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if len(cfg.BackoffSchedule) == 0 {
		cfg.BackoffSchedule = defaultBackoff
	}
	if cfg.stream == nil {
		cfg.stream = LogcatStream
	}
	return &Bridge{cfg: cfg}
}

// Run executes the bridge until ctx is cancelled. Each iteration of
// the outer loop is one subprocess connection: a fresh session id,
// a new logcat subprocess, line-pump until exit, then exponential
// backoff before the next iteration. Returns nil on clean shutdown
// (ctx cancelled). Errors are reserved for unrecoverable conditions
// the caller must surface.
//
// Run owns no goroutines other than its own — the LogcatStream-internal
// goroutines are scoped to a child context derived from ctx, so they
// terminate before the next attempt or before Run returns.
func (b *Bridge) Run(ctx context.Context) error {
	log := b.cfg.Logger.With(
		slog.String("component", "adb-bridge"),
		slog.String("device_serial", b.cfg.DeviceSerial),
	)

	attempt := 0
	for {
		// Honour cancellation up-front so the very first iteration
		// already respects an already-cancelled ctx.
		if err := ctx.Err(); err != nil {
			log.Info("adb bridge stopping", slog.String("reason", "context cancelled"))
			return nil
		}

		sessionID, gotLines, err := b.runOnce(ctx, log)
		switch {
		case err == nil:
			// Subprocess exited cleanly (EOF). Treat as reconnect-
			// worthy: it usually means the adb daemon got restarted
			// or the device unplugged — both transient.
			log.Info("adb logcat subprocess ended", slog.String("session_id", sessionID))
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			log.Info("adb bridge stopping",
				slog.String("session_id", sessionID),
				slog.String("reason", "context cancelled"))
			return nil
		default:
			log.Warn("adb logcat attempt failed",
				slog.String("session_id", sessionID),
				slog.Any("error", err),
			)
		}

		if gotLines {
			// We saw at least one line on this attempt — the
			// subprocess was healthy enough to count as "connected".
			// Reset the backoff so a long-lived adb that drops once
			// a day doesn't end up waiting 10s next time.
			attempt = 0
		}

		wait := b.backoffAt(attempt)
		log.Info("adb bridge backing off before reconnect",
			slog.Duration("wait", wait),
			slog.Int("attempt", attempt+1),
		)
		select {
		case <-ctx.Done():
			log.Info("adb bridge stopping", slog.String("reason", "context cancelled"))
			return nil
		case <-time.After(wait):
		}
		attempt++
	}
}

// backoffAt returns the wait duration for the n-th reconnect attempt.
// Indices past the schedule clamp to the last entry so the bridge
// never overshoots a sane upper bound.
func (b *Bridge) backoffAt(n int) time.Duration {
	sched := b.cfg.BackoffSchedule
	if n >= len(sched) {
		return sched[len(sched)-1]
	}
	return sched[n]
}

// runOnce performs a single connect-stream-disconnect cycle. It opens
// a fresh store session, starts a LogcatStream child, pumps lines into
// the store and the hub until either ctx is cancelled or the line
// channel closes (subprocess exit). Returns:
//   - sessionID: the session id this attempt used (empty if session
//     creation itself failed).
//   - gotLines: true if at least one line made it through the pump.
//     Used by Run to decide whether to reset the backoff counter.
//   - err: nil on clean subprocess exit, ctx error on cancellation,
//     or any startup error from CreateSession / LogcatStream.
func (b *Bridge) runOnce(ctx context.Context, log *slog.Logger) (string, bool, error) {
	// Each reconnect opens a new session so disconnect gaps are
	// visible in the forensic trail (no missing-line guesswork).
	sessionID, err := b.cfg.Store.CreateSession(ctx, "adb-bridge: "+b.deviceLabel())
	if err != nil {
		return "", false, err
	}
	log = log.With(slog.String("session_id", sessionID))
	log.Info("adb bridge starting")

	// Child context lets us tear down the LogcatStream goroutines
	// without cancelling the parent (which would kill the bridge).
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	lines, err := b.cfg.stream(streamCtx, b.cfg.DeviceSerial, b.cfg.TagFilter)
	if err != nil {
		// CreateSession already succeeded, so close it so the row
		// has an ended_at and isn't an open dangling session.
		b.endSession(ctx, sessionID, log)
		return sessionID, false, err
	}

	gotLines, pumpErr := b.pump(ctx, sessionID, lines, log)

	// Drain remaining LogcatStream output (best-effort) so the
	// reader goroutine inside LogcatStream terminates cleanly.
	cancelStream()
	for range lines { // nolint:revive — empty body intended
	}

	b.endSession(ctx, sessionID, log)
	return sessionID, gotLines, pumpErr
}

// pump reads parsed logcat lines, batches them by size+time and writes
// each batch to the store + hub. Returns when the line channel closes
// or ctx is cancelled. The boolean return reports whether any line
// was processed at all (used for backoff reset).
func (b *Bridge) pump(ctx context.Context, sessionID string, lines <-chan LogcatLine, log *slog.Logger) (bool, error) {
	timer := time.NewTimer(b.cfg.BatchInterval)
	defer timer.Stop()

	buf := make([]store.Event, 0, b.cfg.BatchSize)
	gotAny := false
	flush := func() {
		if len(buf) == 0 {
			return
		}
		// Use a detached context for the flush so that we can still
		// persist already-buffered lines during shutdown — once ctx
		// is cancelled, sql operations would otherwise be aborted
		// mid-write. Bound the detached write so we don't hang
		// forever on a wedged DB during shutdown.
		flushCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := b.cfg.Store.InsertEvents(flushCtx, sessionID, buf); err != nil {
			log.Warn("adb event batch insert failed",
				slog.Int("batch_size", len(buf)),
				slog.Any("error", err),
			)
			// Errors do NOT terminate the bridge — analogous to
			// the UpsertCrash robustness in /ingest. We log and
			// drop, then continue pumping.
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return gotAny, ctx.Err()

		case line, ok := <-lines:
			if !ok {
				// Subprocess exited (EOF). Flush remainder
				// and report clean termination so the outer
				// loop reconnects.
				flush()
				return gotAny, nil
			}
			evt, drop := b.lineToEvent(sessionID, line, log)
			if drop {
				continue
			}
			gotAny = true

			// Persist intent: append, conditionally flush, conditionally
			// reset timer. The "size flush" path resets the timer so a
			// burst of lines doesn't fire two flushes back-to-back.
			buf = append(buf, evt.storeEvent)
			b.publish(evt.wsEvent)
			if len(buf) >= b.cfg.BatchSize {
				flush()
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(b.cfg.BatchInterval)
			}

		case <-timer.C:
			flush()
			timer.Reset(b.cfg.BatchInterval)
		}
	}
}

// pendingEvent carries the parallel store/ws representations of one
// logcat line so we can keep their construction in one place.
type pendingEvent struct {
	storeEvent store.Event
	wsEvent    ws.Event
}

// lineToEvent maps a LogcatLine to (store.Event, ws.Event). The boolean
// drop is true when the line should be ignored entirely (level 'S' /
// silent — that's a logcat *filter directive*, not a real log level
// the user wants persisted).
func (b *Bridge) lineToEvent(sessionID string, line LogcatLine, log *slog.Logger) (pendingEvent, bool) {
	level, ok := mapLogcatLevel(line.Level)
	if !ok {
		// 'S' (silent) — ignore. Any other unknown rune also lands
		// here; we log at debug so a future logcat priority gets
		// surfaced rather than silently dropped without trace.
		if line.Level != 'S' {
			log.Debug("adb skipping unknown logcat level",
				slog.String("level_raw", string(line.Level)),
				slog.String("tag", line.Tag),
			)
		}
		return pendingEvent{}, true
	}

	meta := buildMeta(line, b.cfg.DeviceSerial)
	msg := line.Tag + ": " + line.Message
	ts := line.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	return pendingEvent{
		storeEvent: store.Event{
			TS:     ts,
			Source: "adb",
			Level:  level,
			Msg:    msg,
			Meta:   meta,
		},
		wsEvent: ws.Event{
			SessionID: sessionID,
			TS:        ts.UnixNano(),
			Source:    "adb",
			Level:     level,
			Msg:       msg,
			Meta:      meta,
		},
	}, false
}

// publish forwards an event to the hub if one is configured. Hub.Publish
// is non-blocking (drop-on-full) so this never stalls the pump.
func (b *Bridge) publish(evt ws.Event) {
	if b.cfg.Hub == nil {
		return
	}
	b.cfg.Hub.Publish(evt)
}

// endSession is a best-effort EndSession call that swallows errors —
// failing to end an adb session is not worth aborting the bridge for.
func (b *Bridge) endSession(ctx context.Context, sessionID string, log *slog.Logger) {
	if sessionID == "" {
		return
	}
	// Use a short detached timeout so we still close the session even
	// if ctx is already cancelled (shutdown path).
	endCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.cfg.Store.EndSession(endCtx, sessionID); err != nil {
		log.Debug("adb end session failed", slog.Any("error", err))
	}
}

// deviceLabel returns the user-visible serial fragment used in the
// session label. Empty serial → "<auto>" so the label still reads
// sensibly when adb picks the device.
func (b *Bridge) deviceLabel() string {
	if b.cfg.DeviceSerial == "" {
		return "<auto>"
	}
	return b.cfg.DeviceSerial
}

// mapLogcatLevel maps the single-rune logcat priority to the level
// string used by store.Event / ws.Event. Returns ok=false for 'S'
// (silent — not a real level, just a filter directive) and for any
// unknown rune.
func mapLogcatLevel(level rune) (string, bool) {
	switch level {
	case 'V', 'D':
		return "debug", true
	case 'I':
		return "info", true
	case 'W':
		return "warn", true
	case 'E', 'F':
		return "error", true
	default:
		return "", false
	}
}

// buildMeta serialises the per-line metadata block into JSON. Any
// marshal failure falls back to nil meta — the event itself is still
// useful with just msg/level/source.
func buildMeta(line LogcatLine, deviceSerial string) json.RawMessage {
	m := map[string]any{
		"pid":           line.PID,
		"tid":           line.TID,
		"timestamp":     line.Timestamp.Format(time.RFC3339Nano),
		"level_raw":     string(line.Level),
		"device_serial": deviceSerial,
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return raw
}
