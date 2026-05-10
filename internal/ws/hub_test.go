package ws

import (
	"sync"
	"testing"
	"time"
)

// TestHub_SubscribeAllSessions verifies that an unfiltered subscriber
// receives events from any session.
func TestHub_SubscribeAllSessions(t *testing.T) {
	h := NewHub(8)
	defer h.Close()

	ch, cancel := h.Subscribe("")
	defer cancel()

	h.Publish(Event{SessionID: "s1", Msg: "a"})
	h.Publish(Event{SessionID: "s2", Msg: "b"})

	got := drain(t, ch, 2, 200*time.Millisecond)
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].SessionID != "s1" || got[1].SessionID != "s2" {
		t.Fatalf("unexpected session order: %+v", got)
	}
}

// TestHub_SubscribeWithFilter verifies that a session-filtered subscriber
// only sees events from its session.
func TestHub_SubscribeWithFilter(t *testing.T) {
	h := NewHub(8)
	defer h.Close()

	ch, cancel := h.Subscribe("s1")
	defer cancel()

	h.Publish(Event{SessionID: "s1", Msg: "a"})
	h.Publish(Event{SessionID: "s2", Msg: "b"})
	h.Publish(Event{SessionID: "s1", Msg: "c"})

	got := drain(t, ch, 2, 200*time.Millisecond)
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	for _, e := range got {
		if e.SessionID != "s1" {
			t.Fatalf("leaked session %q", e.SessionID)
		}
	}
}

// TestHub_Unsubscribe verifies that calling cancel removes the subscriber
// and closes its channel.
func TestHub_Unsubscribe(t *testing.T) {
	h := NewHub(8)
	defer h.Close()

	ch, cancel := h.Subscribe("")
	if got := h.SubscriberCount(); got != 1 {
		t.Fatalf("subs after subscribe = %d, want 1", got)
	}
	cancel()
	if got := h.SubscriberCount(); got != 0 {
		t.Fatalf("subs after cancel = %d, want 0", got)
	}
	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel still open after cancel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed within timeout")
	}
	// Idempotent — second cancel must not panic.
	cancel()
}

// TestHub_Close verifies that all subscribers are closed and Done() fires.
func TestHub_Close(t *testing.T) {
	h := NewHub(8)
	ch1, _ := h.Subscribe("")
	ch2, _ := h.Subscribe("session-x")

	h.Close()
	// All subscriber channels must be closed.
	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatalf("ch%d still open after Close", i+1)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("ch%d not closed within timeout", i+1)
		}
	}
	select {
	case <-h.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Done() did not fire after Close")
	}
	// Subscribe-after-close returns a closed channel and a no-op cancel.
	ch3, cancel := h.Subscribe("")
	cancel()
	if _, ok := <-ch3; ok {
		t.Fatal("subscribe-after-close returned an open channel")
	}
	// Close is idempotent.
	h.Close()
}

// TestHub_PublishNonBlockingOnFull asserts that a slow subscriber does not
// stall the publisher: events are dropped for the slow subscriber while
// the publisher and other subscribers continue.
func TestHub_PublishNonBlockingOnFull(t *testing.T) {
	h := NewHub(2) // tiny buffer
	defer h.Close()

	slow, cancelSlow := h.Subscribe("")
	defer cancelSlow()
	fast, cancelFast := h.Subscribe("")
	defer cancelFast()

	// Drain `fast` continuously; leave `slow` untouched so it fills up.
	var fastCount int
	var fastMu sync.Mutex
	done := make(chan struct{})
	go func() {
		for range fast {
			fastMu.Lock()
			fastCount++
			fastMu.Unlock()
		}
		close(done)
	}()

	// Publish more than buf-size events; must not block.
	const N = 100
	pubDone := make(chan struct{})
	go func() {
		for i := 0; i < N; i++ {
			h.Publish(Event{SessionID: "s", Msg: "x"})
		}
		close(pubDone)
	}()
	select {
	case <-pubDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Publish blocked on full subscriber")
	}

	// Slow subscriber's buffer should be at capacity (2) but not more.
	if got := len(slow); got > 2 {
		t.Fatalf("slow buffer = %d, want <=2", got)
	}

	// Stop fast subscriber via cancel and wait for goroutine to drain.
	cancelFast()
	<-done

	fastMu.Lock()
	defer fastMu.Unlock()
	if fastCount == 0 {
		t.Fatal("fast subscriber received zero events")
	}
}

// TestHub_RaceParallelPubSub stresses Publish/Subscribe/Unsubscribe under
// `go test -race` to surface data races.
func TestHub_RaceParallelPubSub(t *testing.T) {
	h := NewHub(16)
	defer h.Close()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 4 publisher goroutines.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					h.Publish(Event{SessionID: "s", Msg: "x"})
				}
			}
		}()
	}

	// 4 subscribe/unsubscribe churn goroutines.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					ch, cancel := h.Subscribe("")
					// Drain a few then cancel.
					for j := 0; j < 5; j++ {
						select {
						case <-ch:
						case <-time.After(2 * time.Millisecond):
						}
					}
					cancel()
				}
			}
		}()
	}

	time.Sleep(150 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// drain reads up to n events from ch with a per-event timeout, returning
// what it got (which may be fewer than n).
func drain(t *testing.T, ch <-chan Event, n int, timeout time.Duration) []Event {
	t.Helper()
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		select {
		case e, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, e)
		case <-time.After(timeout):
			return out
		}
	}
	return out
}
