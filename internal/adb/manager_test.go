package adb

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// stubManagedFactory builds the manager's BridgeFactory hook: each call
// returns a real *Bridge whose stream is wired to an immediately-blocking
// goroutine that exits when its child context is cancelled. The bridge's
// Run loop therefore behaves like a long-running production bridge with no
// I/O — perfect for lifecycle assertions in the manager test.
//
// startedCounter is incremented once per Bridge.Run invocation; useful for
// asserting "the goroutine actually entered Run" without time.Sleep.
type stubManagedFactory struct {
	startedCounter atomic.Int32
	stoppedCounter atomic.Int32

	mu     sync.Mutex
	stubs  map[string]chan struct{} // serial -> "stream cancelled" signal
}

func newStubManagedFactory() *stubManagedFactory {
	return &stubManagedFactory{stubs: make(map[string]chan struct{})}
}

// factory returns a BridgeFactory closure suitable for BridgeManagerDeps.
// It builds a Bridge whose stream produces zero lines but blocks until
// the stream context is cancelled — i.e. emulates a healthy logcat
// subprocess that is silent for the duration of the test.
func (s *stubManagedFactory) factory() BridgeFactory {
	return func(cfg BridgeConfig) *Bridge {
		serial := cfg.DeviceSerial
		cancelled := make(chan struct{})
		s.mu.Lock()
		s.stubs[serial] = cancelled
		s.mu.Unlock()

		cfg.stream = func(ctx context.Context, _, _ string) (<-chan LogcatLine, error) {
			s.startedCounter.Add(1)
			ch := make(chan LogcatLine)
			go func() {
				defer close(ch)
				<-ctx.Done()
				s.stoppedCounter.Add(1)
				// Signal the test that the stream context was honoured.
				select {
				case <-cancelled:
				default:
					close(cancelled)
				}
			}()
			return ch, nil
		}
		// Tight backoff so the reconnect loop wakes quickly when the
		// manager cancels the outer ctx.
		if cfg.BackoffSchedule == nil {
			cfg.BackoffSchedule = []time.Duration{
				1 * time.Millisecond,
				1 * time.Millisecond,
			}
		}
		// Tiny batch interval so the pump doesn't park for the default
		// 50 ms when we're cancelling.
		if cfg.BatchInterval == 0 {
			cfg.BatchInterval = time.Millisecond
		}
		return NewBridge(cfg)
	}
}

func (s *stubManagedFactory) cancelSignal(serial string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stubs[serial]
}

// waitStarted polls startedCounter until it reaches n or the deadline fires.
func (s *stubManagedFactory) waitStarted(t *testing.T, n int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.startedCounter.Load() >= n {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("expected at least %d Run invocations, got %d", n, s.startedCounter.Load())
}

func TestManager_Start_NewSerial(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	if m == nil {
		t.Fatal("NewBridgeManager returned nil")
	}
	defer m.Close()

	status, err := m.Start(BridgeStartOptions{DeviceSerial: "emu-1"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if status.DeviceSerial != "emu-1" {
		t.Errorf("Status serial = %q", status.DeviceSerial)
	}
	if status.StartedAt == 0 {
		t.Errorf("StartedAt unset")
	}
	if !m.IsRunning("emu-1") {
		t.Errorf("manager does not report emu-1 as running")
	}
	stub.waitStarted(t, 1)
}

func TestManager_Start_AlreadyRunning_Idempotent(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	defer m.Close()

	first, err := m.Start(BridgeStartOptions{DeviceSerial: "emu-2"})
	if err != nil {
		t.Fatalf("first Start: %v", err)
	}

	second, err := m.Start(BridgeStartOptions{DeviceSerial: "emu-2"})
	if !errors.Is(err, ErrBridgeAlreadyRunning) {
		t.Fatalf("expected ErrBridgeAlreadyRunning, got %v", err)
	}
	if second.StartedAt != first.StartedAt {
		t.Errorf("idempotent Start must echo first StartedAt: got %d, want %d", second.StartedAt, first.StartedAt)
	}
}

func TestManager_Stop_RunningBridge(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	defer m.Close()

	if _, err := m.Start(BridgeStartOptions{DeviceSerial: "emu-3"}); err != nil {
		t.Fatal(err)
	}
	stub.waitStarted(t, 1)

	if err := m.Stop("emu-3"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.IsRunning("emu-3") {
		t.Errorf("manager still reports emu-3 as running after Stop")
	}
	if stub.stoppedCounter.Load() < 1 {
		t.Errorf("stream cancel signal never observed: stopped=%d", stub.stoppedCounter.Load())
	}
}

func TestManager_Stop_UnknownSerial(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	defer m.Close()

	err := m.Stop("ghost")
	if !errors.Is(err, ErrBridgeNotRunning) {
		t.Errorf("expected ErrBridgeNotRunning, got %v", err)
	}
}

func TestManager_Start_EmptySerial(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	defer m.Close()

	_, err := m.Start(BridgeStartOptions{DeviceSerial: ""})
	if err == nil {
		t.Fatal("expected error for empty serial")
	}
}

func TestManager_StartStopStart_SameSerial(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	defer m.Close()

	if _, err := m.Start(BridgeStartOptions{DeviceSerial: "emu-4"}); err != nil {
		t.Fatal(err)
	}
	stub.waitStarted(t, 1)

	if err := m.Stop("emu-4"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Re-start must succeed.
	if _, err := m.Start(BridgeStartOptions{DeviceSerial: "emu-4"}); err != nil {
		t.Fatalf("re-Start: %v", err)
	}
	stub.waitStarted(t, 2)
}

func TestManager_Status_ListsRunning(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	defer m.Close()

	for _, s := range []string{"emu-a", "emu-b", "emu-c"} {
		if _, err := m.Start(BridgeStartOptions{DeviceSerial: s}); err != nil {
			t.Fatalf("Start %s: %v", s, err)
		}
	}
	got := m.Status()
	if len(got) != 3 {
		t.Fatalf("Status len = %d, want 3", len(got))
	}
	seen := map[string]bool{}
	for _, s := range got {
		seen[s.DeviceSerial] = true
	}
	for _, want := range []string{"emu-a", "emu-b", "emu-c"} {
		if !seen[want] {
			t.Errorf("Status missing %s", want)
		}
	}
}

func TestManager_Close_StopsAllBridges(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})

	for _, s := range []string{"emu-x", "emu-y"} {
		if _, err := m.Start(BridgeStartOptions{DeviceSerial: s}); err != nil {
			t.Fatalf("Start %s: %v", s, err)
		}
	}
	stub.waitStarted(t, 2)
	m.Close()

	// Both stream-cancel signals must have fired.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if stub.stoppedCounter.Load() >= 2 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if stub.stoppedCounter.Load() < 2 {
		t.Errorf("expected 2 stream cancels, got %d", stub.stoppedCounter.Load())
	}
	// Double-close is safe.
	m.Close()
}

func TestManager_ConcurrentStartStop_SameSerial(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	defer m.Close()

	// Hammer Start+Stop on the same serial from multiple goroutines. The
	// invariants are: no panic, no goroutine leak, and the post-condition
	// map is consistent.
	const workers = 8
	const iter = 25
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iter; j++ {
				_, _ = m.Start(BridgeStartOptions{DeviceSerial: "race"})
				_ = m.Stop("race")
			}
		}()
	}
	wg.Wait()
}

func TestNewBridgeManager_NilStore(t *testing.T) {
	t.Parallel()
	m := NewBridgeManager(BridgeManagerDeps{})
	if m != nil {
		t.Errorf("expected nil manager when Store is unset, got %v", m)
	}
}

func TestManager_AfterClose_StartFails(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	stub := newStubManagedFactory()
	m := NewBridgeManager(BridgeManagerDeps{Store: st, Factory: stub.factory()})
	m.Close()

	_, err := m.Start(BridgeStartOptions{DeviceSerial: "x"})
	if err == nil {
		t.Fatal("expected error starting on closed manager")
	}
}
