package adb

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VibeCodeSolutions/tracelab/internal/store"
	"github.com/VibeCodeSolutions/tracelab/internal/ws"
)

// fakeStore is a thread-safe in-memory BridgeStore used in bridge tests.
// It mirrors the bits of *store.Store the bridge actually exercises:
// CreateSession returns a fresh id, InsertEvents records the rows, and
// EndSession marks the session as closed.
type fakeStore struct {
	mu        sync.Mutex
	sessions  []string                  // creation order
	ended     map[string]bool           // session id -> ended?
	events    map[string][]store.Event  // session id -> events
	nextIDInt int
	// failCreate, if non-nil, is returned from CreateSession (one-shot).
	failCreate error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		ended:  map[string]bool{},
		events: map[string][]store.Event{},
	}
}

func (f *fakeStore) CreateSession(_ context.Context, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failCreate; err != nil {
		f.failCreate = nil
		return "", err
	}
	f.nextIDInt++
	id := "sess-" + itoa(f.nextIDInt)
	f.sessions = append(f.sessions, id)
	f.events[id] = nil
	return id, nil
}

func (f *fakeStore) InsertEvents(_ context.Context, sessionID string, events []store.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]store.Event, len(events))
	copy(cp, events)
	f.events[sessionID] = append(f.events[sessionID], cp...)
	return nil
}

func (f *fakeStore) EndSession(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.events[id]; !ok {
		return errors.New("unknown session")
	}
	f.ended[id] = true
	return nil
}

func (f *fakeStore) snapshot() ([]string, map[string][]store.Event, map[string]bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sess := append([]string(nil), f.sessions...)
	ev := make(map[string][]store.Event, len(f.events))
	for k, v := range f.events {
		ev[k] = append([]store.Event(nil), v...)
	}
	en := make(map[string]bool, len(f.ended))
	for k, v := range f.ended {
		en[k] = v
	}
	return sess, ev, en
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}

// fakeHub captures Publish calls for assertion. Implements BridgePublisher.
type fakeHub struct {
	mu     sync.Mutex
	events []ws.Event
}

func (h *fakeHub) Publish(e ws.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e)
}

func (h *fakeHub) snapshot() []ws.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]ws.Event(nil), h.events...)
}

// staticStream returns a streamFunc that emits the given lines once
// and then closes. Each call to the returned function (i.e. each
// reconnect) consumes one entry from streams; if streams runs out,
// the stream returns the err value (or, if err is nil, an empty
// already-closed channel — simulating an immediate EOF).
func staticStream(streams [][]LogcatLine, errs []error, started *int32) streamFunc {
	var idx atomic.Int32
	return func(ctx context.Context, _, _ string) (<-chan LogcatLine, error) {
		i := int(idx.Add(1) - 1)
		if started != nil {
			atomic.AddInt32(started, 1)
		}
		var lines []LogcatLine
		var err error
		if i < len(streams) {
			lines = streams[i]
		}
		if i < len(errs) {
			err = errs[i]
		}
		if err != nil {
			return nil, err
		}
		ch := make(chan LogcatLine, len(lines)+1)
		go func() {
			defer close(ch)
			for _, l := range lines {
				select {
				case <-ctx.Done():
					return
				case ch <- l:
				}
			}
		}()
		return ch, nil
	}
}

// fastBackoff returns a near-zero backoff schedule for tests so the
// reconnect loop fires within the test budget.
func fastBackoff() []time.Duration {
	return []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		5 * time.Millisecond,
	}
}

// -----------------------------------------------------------------------
// Level mapping
// -----------------------------------------------------------------------

func TestMapLogcatLevel(t *testing.T) {
	cases := []struct {
		in   rune
		want string
		ok   bool
	}{
		{'V', "debug", true},
		{'D', "debug", true},
		{'I', "info", true},
		{'W', "warn", true},
		{'E', "error", true},
		{'F', "error", true},
		{'S', "", false},
		{'X', "", false}, // unknown
	}
	for _, tc := range cases {
		got, ok := mapLogcatLevel(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("mapLogcatLevel(%q)=%q,%v want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// TestBridge_LineFlow_AllLevels feeds one line per level (V/D/I/W/E/F/S)
// through the bridge and verifies persistence + fan-out, including S
// being silently dropped.
func TestBridge_LineFlow_AllLevels(t *testing.T) {
	st := newFakeStore()
	hub := &fakeHub{}

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	lines := []LogcatLine{
		{Timestamp: now, PID: 1, TID: 2, Level: 'V', Tag: "T", Message: "verbose"},
		{Timestamp: now, PID: 1, TID: 2, Level: 'D', Tag: "T", Message: "debug"},
		{Timestamp: now, PID: 1, TID: 2, Level: 'I', Tag: "T", Message: "info"},
		{Timestamp: now, PID: 1, TID: 2, Level: 'W', Tag: "T", Message: "warn"},
		{Timestamp: now, PID: 1, TID: 2, Level: 'E', Tag: "T", Message: "error"},
		{Timestamp: now, PID: 1, TID: 2, Level: 'F', Tag: "T", Message: "fatal"},
		{Timestamp: now, PID: 1, TID: 2, Level: 'S', Tag: "T", Message: "silent-skip"},
	}

	br := NewBridge(BridgeConfig{
		DeviceSerial:    "emulator-5554",
		Store:           st,
		Hub:             hub,
		BatchInterval:   5 * time.Millisecond,
		BatchSize:       100,
		BackoffSchedule: fastBackoff(),
		stream:          staticStream([][]LogcatLine{lines}, nil, nil),
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- br.Run(ctx) }()

	// Wait until events are visible and pump idled past stream-EOF
	// (the bridge will then enter backoff). Then cancel.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, ev, _ := st.snapshot()
		total := 0
		for _, slice := range ev {
			total += len(slice)
		}
		if total >= 6 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("bridge run returned err: %v", err)
	}

	sessions, ev, ended := st.snapshot()
	if len(sessions) < 1 {
		t.Fatalf("expected at least 1 session, got %d", len(sessions))
	}
	first := sessions[0]
	got := ev[first]
	if len(got) != 6 {
		t.Fatalf("want 6 persisted events (S dropped), got %d", len(got))
	}
	wantLevels := []string{"debug", "debug", "info", "warn", "error", "error"}
	wantMsgs := []string{
		"T: verbose",
		"T: debug",
		"T: info",
		"T: warn",
		"T: error",
		"T: fatal",
	}
	for i, e := range got {
		if e.Level != wantLevels[i] {
			t.Errorf("event[%d] level=%q want %q", i, e.Level, wantLevels[i])
		}
		if e.Source != "adb" {
			t.Errorf("event[%d] source=%q want adb", i, e.Source)
		}
		if e.Msg != wantMsgs[i] {
			t.Errorf("event[%d] msg=%q want %q", i, e.Msg, wantMsgs[i])
		}
		var meta map[string]any
		if err := json.Unmarshal(e.Meta, &meta); err != nil {
			t.Fatalf("meta unmarshal: %v", err)
		}
		if meta["device_serial"] != "emulator-5554" {
			t.Errorf("meta device_serial=%v", meta["device_serial"])
		}
		if meta["level_raw"] == "" {
			t.Errorf("meta level_raw empty")
		}
	}
	if !ended[first] {
		t.Errorf("first session not ended on shutdown")
	}

	// ws hub fan-out should match persisted count.
	wsEv := hub.snapshot()
	if len(wsEv) != 6 {
		t.Fatalf("want 6 ws events, got %d", len(wsEv))
	}
	for _, e := range wsEv {
		if e.SessionID != first {
			t.Errorf("ws event session=%q want %q", e.SessionID, first)
		}
	}
}

// TestBridge_Reconnect_NewSession verifies that a subprocess-EOF makes
// the bridge spin up a new session and the second batch of lines lands
// in that new session. Backoff is shrunk to milliseconds so the test
// stays well under a second.
func TestBridge_Reconnect_NewSession(t *testing.T) {
	st := newFakeStore()

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	first := []LogcatLine{
		{Timestamp: now, PID: 1, TID: 1, Level: 'I', Tag: "A", Message: "first-1"},
		{Timestamp: now, PID: 1, TID: 1, Level: 'I', Tag: "A", Message: "first-2"},
	}
	second := []LogcatLine{
		{Timestamp: now, PID: 2, TID: 2, Level: 'W', Tag: "B", Message: "second-1"},
	}

	var started int32
	br := NewBridge(BridgeConfig{
		DeviceSerial:    "abc123",
		Store:           st,
		BatchInterval:   2 * time.Millisecond,
		BatchSize:       100,
		BackoffSchedule: fastBackoff(),
		stream:          staticStream([][]LogcatLine{first, second}, nil, &started),
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- br.Run(ctx) }()

	// Wait until both stream attempts have fired *and* we've seen
	// 3 total events persisted.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&started) >= 2 {
			_, ev, _ := st.snapshot()
			total := 0
			for _, slice := range ev {
				total += len(slice)
			}
			if total >= 3 {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("bridge run returned err: %v", err)
	}

	sessions, events, _ := st.snapshot()
	if len(sessions) < 2 {
		t.Fatalf("expected ≥2 sessions after reconnect, got %d (%v)", len(sessions), sessions)
	}
	if len(events[sessions[0]]) != 2 {
		t.Errorf("session[0] has %d events, want 2", len(events[sessions[0]]))
	}
	if len(events[sessions[1]]) < 1 {
		t.Errorf("session[1] has %d events, want ≥1", len(events[sessions[1]]))
	}
	// Sanity: the two sessions' messages match the input split.
	if msg := events[sessions[0]][0].Msg; msg != "A: first-1" {
		t.Errorf("session[0][0].msg=%q want A: first-1", msg)
	}
	if msg := events[sessions[1]][0].Msg; msg != "B: second-1" {
		t.Errorf("session[1][0].msg=%q want B: second-1", msg)
	}
}

// TestBridge_BackoffBetweenAttempts checks that the bridge waits the
// configured backoff between two no-line stream attempts when no lines
// were seen.
func TestBridge_BackoffBetweenAttempts(t *testing.T) {
	st := newFakeStore()

	var started int32
	// Three immediate-EOF streams.
	br := NewBridge(BridgeConfig{
		DeviceSerial:    "x",
		Store:           st,
		BatchInterval:   2 * time.Millisecond,
		BatchSize:       10,
		BackoffSchedule: []time.Duration{20 * time.Millisecond, 40 * time.Millisecond},
		stream:          staticStream([][]LogcatLine{nil, nil, nil}, nil, &started),
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- br.Run(ctx) }()

	// Two attempts must take at least 20ms (first backoff). We also
	// allow plenty of headroom on slow CI.
	t0 := time.Now()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&started) >= 2 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	elapsed := time.Since(t0)
	cancel()
	<-done

	if atomic.LoadInt32(&started) < 2 {
		t.Fatalf("only %d attempts started", started)
	}
	if elapsed < 15*time.Millisecond {
		t.Errorf("two attempts completed in %v — backoff appears not to be applied", elapsed)
	}
}

// TestBridge_StreamErrorFails attempt yields error, bridge should treat
// it as a failed attempt, end the session, and retry.
func TestBridge_StreamErrorFails(t *testing.T) {
	st := newFakeStore()

	var started int32
	br := NewBridge(BridgeConfig{
		Store:           st,
		BatchInterval:   2 * time.Millisecond,
		BatchSize:       10,
		BackoffSchedule: fastBackoff(),
		stream: staticStream(
			[][]LogcatLine{nil, {{Timestamp: time.Now(), Level: 'I', Tag: "T", Message: "ok"}}},
			[]error{errors.New("boom"), nil},
			&started,
		),
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- br.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&started) >= 2 {
			_, ev, _ := st.snapshot()
			total := 0
			for _, s := range ev {
				total += len(s)
			}
			if total >= 1 {
				break
			}
		}
		time.Sleep(1 * time.Millisecond)
	}
	cancel()
	<-done

	sessions, events, ended := st.snapshot()
	if len(sessions) < 2 {
		t.Fatalf("want ≥2 sessions (one for failed start, one for success), got %d", len(sessions))
	}
	// The failed-start session must still have been ended.
	if !ended[sessions[0]] {
		t.Errorf("failed-start session not ended")
	}
	// The successful attempt must have at least one event in its session.
	found := false
	for _, sid := range sessions[1:] {
		if len(events[sid]) >= 1 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ≥1 event in a post-error session")
	}
}

// TestBridge_ContextCancelStops cancels mid-stream and ensures Run returns
// promptly without leaving the session open.
func TestBridge_ContextCancelStops(t *testing.T) {
	st := newFakeStore()

	now := time.Now()
	lines := make([]LogcatLine, 3)
	for i := range lines {
		lines[i] = LogcatLine{Timestamp: now, PID: 1, TID: 1, Level: 'I', Tag: "X", Message: "m"}
	}
	br := NewBridge(BridgeConfig{
		Store:           st,
		BatchInterval:   2 * time.Millisecond,
		BatchSize:       10,
		BackoffSchedule: fastBackoff(),
		stream:          staticStream([][]LogcatLine{lines}, nil, nil),
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- br.Run(ctx) }()

	// Give it a tick to ingest at least one line, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within 2s of cancel")
	}

	sessions, _, ended := st.snapshot()
	if len(sessions) == 0 {
		t.Fatalf("no session created")
	}
	for _, s := range sessions {
		if !ended[s] {
			t.Errorf("session %q not ended on shutdown", s)
		}
	}
}
