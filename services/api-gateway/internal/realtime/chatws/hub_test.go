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

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < defaultQueueSize+16; i++ {
			h.Broadcast([]string{"u1", "u2"}, map[string]any{"n": i})
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("broadcast loop blocked by slow client")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for fast.writes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if fast.writes.Load() == 0 {
		t.Fatal("expected fast client to receive messages")
	}
}

