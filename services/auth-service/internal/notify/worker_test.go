package notify

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

func newTestWorker() *TelegramWorker {
	// nil Telegram client: send() returns at the !Enabled() guard, so no network
	// is touched. This lets us exercise the buffering / lifecycle logic in isolation.
	return NewTelegramWorker(nil, "https://app.example", zerolog.Nop())
}

func TestNewTelegramWorker_BufferCapacity(t *testing.T) {
	w := newTestWorker()
	if cap(w.ch) != defaultBufSize {
		t.Errorf("channel capacity = %d, want %d", cap(w.ch), defaultBufSize)
	}
}

func TestEnqueue_DoesNotBlockWhenBufferFull(t *testing.T) {
	w := newTestWorker()

	// Fill the buffer exactly, then overflow it. No worker is draining, so the
	// overflow events must be dropped rather than block the caller.
	done := make(chan struct{})
	go func() {
		for i := 0; i < defaultBufSize+10; i++ {
			w.Enqueue(domain.PartnerRegisteredEvent{Role: "master", UserID: "u"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked when buffer was full — caller (Register RPC) must never block")
	}

	if got := len(w.ch); got != defaultBufSize {
		t.Errorf("buffered events = %d, want %d (overflow dropped)", got, defaultBufSize)
	}
}

func TestRun_DrainsAndExitsOnCancel(t *testing.T) {
	w := newTestWorker()

	// Pre-load some events; Run must consume them and then return after cancel.
	for i := 0; i < 5; i++ {
		w.Enqueue(domain.PartnerRegisteredEvent{Role: "venue_owner", UserID: "u"})
	}

	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(exited)
	}()

	cancel()

	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not exit within timeout after ctx cancellation")
	}

	if got := len(w.ch); got != 0 {
		t.Errorf("buffer not drained on shutdown: %d events left", got)
	}
}

func TestRun_ProcessesEnqueuedEvents(t *testing.T) {
	w := newTestWorker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	for i := 0; i < 3; i++ {
		w.Enqueue(domain.PartnerRegisteredEvent{Role: "master", UserID: "u"})
	}

	// The single worker goroutine should drain the buffer back to empty.
	deadline := time.After(2 * time.Second)
	for {
		if len(w.ch) == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("worker did not drain enqueued events: %d left", len(w.ch))
		case <-time.After(10 * time.Millisecond):
		}
	}
}
