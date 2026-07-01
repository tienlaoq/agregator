package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSlidingWindow_Allow_WithinLimit(t *testing.T) {
	l := New()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ok, err := l.Allow(ctx, "k", 3, time.Minute)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("call %d: expected allowed within limit", i)
		}
	}
}

func TestSlidingWindow_Allow_BlocksOverLimit(t *testing.T) {
	l := New()
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow(ctx, "k", 2, time.Minute); !ok {
			t.Fatalf("call %d should be allowed", i)
		}
	}
	if ok, _ := l.Allow(ctx, "k", 2, time.Minute); ok {
		t.Fatal("third call should be blocked once limit reached")
	}
}

func TestSlidingWindow_Allow_NonPositiveLimit(t *testing.T) {
	l := New()
	ctx := context.Background()
	for _, limit := range []int{0, -1} {
		if ok, err := l.Allow(ctx, "k", limit, time.Minute); err != nil || !ok {
			t.Fatalf("limit %d: expected allow with no error, got ok=%v err=%v", limit, ok, err)
		}
	}
}

func TestSlidingWindow_Allow_WindowExpiry(t *testing.T) {
	l := New()
	ctx := context.Background()
	window := 30 * time.Millisecond

	if ok, _ := l.Allow(ctx, "k", 1, window); !ok {
		t.Fatal("first call should be allowed")
	}
	if ok, _ := l.Allow(ctx, "k", 1, window); ok {
		t.Fatal("immediate second call should be blocked")
	}
	time.Sleep(window + 10*time.Millisecond)
	if ok, _ := l.Allow(ctx, "k", 1, window); !ok {
		t.Fatal("call after window should be allowed again")
	}
}

func TestSlidingWindow_Allow_KeysAreIndependent(t *testing.T) {
	l := New()
	ctx := context.Background()
	if ok, _ := l.Allow(ctx, "a", 1, time.Minute); !ok {
		t.Fatal("key a first call should be allowed")
	}
	if ok, _ := l.Allow(ctx, "b", 1, time.Minute); !ok {
		t.Fatal("key b must not be affected by key a")
	}
	if ok, _ := l.Allow(ctx, "a", 1, time.Minute); ok {
		t.Fatal("key a should now be blocked")
	}
}

// TestSlidingWindow_ZeroValue verifies the documented contract that the zero
// value is usable without calling New (Allow lazily initialises the map).
func TestSlidingWindow_ZeroValue(t *testing.T) {
	var l SlidingWindow
	if ok, err := l.Allow(context.Background(), "k", 1, time.Minute); err != nil || !ok {
		t.Fatalf("zero value should allow first call, got ok=%v err=%v", ok, err)
	}
}

func TestSlidingWindow_Allow_Concurrent(t *testing.T) {
	l := New()
	ctx := context.Background()
	const goroutines = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow(ctx, "shared", 10, time.Minute); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 10 {
		t.Fatalf("expected exactly 10 allowed under a limit of 10, got %d", allowed)
	}
}

func TestNewWithPrune_EvictsStaleKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := NewWithPrune(ctx, 10*time.Millisecond)

	// Record an entry, then prune directly with a zero horizon so the key is
	// considered fully stale and removed.
	if ok, _ := l.Allow(ctx, "stale", 5, time.Minute); !ok {
		t.Fatal("first call should be allowed")
	}
	l.prune(0)

	l.mu.Lock()
	_, present := l.byK["stale"]
	l.mu.Unlock()
	if present {
		t.Fatal("prune(0) should have evicted the stale key")
	}
}

func TestPrune_KeepsActiveKeys(t *testing.T) {
	l := New()
	ctx := context.Background()
	if ok, _ := l.Allow(ctx, "fresh", 5, time.Minute); !ok {
		t.Fatal("first call should be allowed")
	}
	// Horizon far in the past: the just-added timestamp is newer than cutoff,
	// so the key must survive.
	l.prune(time.Hour)

	l.mu.Lock()
	_, present := l.byK["fresh"]
	l.mu.Unlock()
	if !present {
		t.Fatal("prune must not evict keys with recent timestamps")
	}
}

func TestRedisLimiter_Allow_NoopGuards(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		l      *RedisLimiter
		limit  int
		window time.Duration
	}{
		{"nil receiver", nil, 5, time.Minute},
		{"nil client", &RedisLimiter{}, 5, time.Minute},
		{"non-positive limit", NewRedisLimiter(nil), 0, time.Minute},
		{"non-positive window", &RedisLimiter{}, 5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := tc.l.Allow(ctx, "k", tc.limit, tc.window)
			if err != nil || !ok {
				t.Fatalf("guard path should allow without error, got ok=%v err=%v", ok, err)
			}
		})
	}
}

// RedisLimiter satisfies the Limiter interface; this is a compile-time check
// plus a guard-path exercise through the interface.
func TestRedisLimiter_SatisfiesLimiter(t *testing.T) {
	var _ Limiter = (*RedisLimiter)(nil)
	var _ Limiter = (*SlidingWindow)(nil)
}

func TestRunPrune_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	l := New()
	done := make(chan struct{})
	go func() {
		l.runPrune(ctx, time.Millisecond)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPrune did not return after context cancellation")
	}
}
