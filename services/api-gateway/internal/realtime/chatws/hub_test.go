package chatws

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stubConn struct {
	blockWrites bool
	closeCh     chan struct{}
	closed      atomic.Bool
	writes      atomic.Int32
}

func newStubConn(blockWrites bool) *stubConn {
	return &stubConn{
		blockWrites: blockWrites,
		closeCh:     make(chan struct{}),
	}
}

func (s *stubConn) SetWriteDeadline(_ time.Time) error { return nil }

func (s *stubConn) WriteMessage(_ int, _ []byte) error {
	if s.closed.Load() {
		return errors.New("closed")
	}
	if s.blockWrites {
		<-s.closeCh
		return errors.New("closed")
	}
	s.writes.Add(1)
	return nil
}

func (s *stubConn) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		close(s.closeCh)
	}
	return nil
}

func TestBroadcast_DropsSlowClientWithoutBlocking(t *testing.T) {
	h := NewHub()
	slow := newStubConn(true)
	h.addWithConn("u1", slow)

	start := time.Now()
	for i := 0; i < defaultQueueSize+32; i++ {
		h.Broadcast([]string{"u1"}, map[string]any{"n": i})
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Fatalf("broadcast is unexpectedly slow: %v", elapsed)
	}
	if !slow.closed.Load() {
		t.Fatal("expected slow client to be closed after queue overflow")
	}
}

func TestBroadcast_SlowClientDoesNotBlockOthers(t *testing.T) {
	h := NewHub()
	slow := newStubConn(true)
	fast := newStubConn(false)
	h.addWithConn("u1", slow)
	h.addWithConn("u2", fast)

	// Keep the batch well under defaultQueueSize so neither client overflows
	// its send buffer. Overflow would drop a client via Broadcast's non-blocking
	// `default` case — and on a loaded CI runner the *fast* client's writeLoop
	// may not be scheduled before its buffer fills, dropping it too and making
	// this test flaky. The invariant under test is "a client blocked inside
	// WriteMessage (slow) must not prevent another client (fast) from receiving
	// broadcasts", which holds without forcing an overflow: each client has its
	// own goroutine + buffered channel, and Broadcast never blocks.
	const n = 8
	if n >= defaultQueueSize {
		t.Fatalf("test batch %d must stay below queue size %d", n, defaultQueueSize)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			h.Broadcast([]string{"u1", "u2"}, map[string]any{"n": i})
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// The broadcast loop must finish promptly: a blocked slow client must never
	// stall it. The timeout is generous relative to the multi-minute CI test
	// timeout; it only guards against a genuine deadlock.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("broadcast loop blocked by slow client")
	}

	// The fast client is never dropped (batch < buffer), so its writeLoop will
	// drain all n messages once scheduled. Poll until it catches up.
	deadline := time.Now().Add(5 * time.Second)
	for fast.writes.Load() < int32(n) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := fast.writes.Load(); got < int32(n) {
		t.Fatalf("expected fast client to receive %d messages, got %d", n, got)
	}
	// Sanity: the slow client stayed blocked (not dropped) — we never overflowed.
	if slow.closed.Load() {
		t.Fatal("slow client was unexpectedly dropped; batch overflowed the queue")
	}
}

