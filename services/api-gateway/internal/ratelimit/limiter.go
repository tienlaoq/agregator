// Package ratelimit provides a thread-safe in-memory sliding-window rate limiter
// shared by handler and middleware packages inside the api-gateway service.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// SlidingWindow is a thread-safe in-memory sliding-window rate limiter.
// It tracks the timestamps of recent Allow calls per key and returns false
// once the count within the window reaches the limit.
//
// The zero value is valid and ready to use without a prune goroutine.
// Use NewWithPrune when long-lived keys should be evicted automatically.
type SlidingWindow struct {
	mu  sync.Mutex
	byK map[string][]time.Time
}

// New returns a SlidingWindow with no background prune goroutine.
// Stale timestamps are trimmed lazily on each Allow call, so memory grows
// only up to the number of distinct active keys. Suitable for low-cardinality
// key spaces (e.g. per-IP forgot-password limiting).
func New() *SlidingWindow {
	return &SlidingWindow{byK: make(map[string][]time.Time)}
}

// NewWithPrune returns a SlidingWindow with a background goroutine that
// evicts fully-stale keys every pruneInterval.  The goroutine stops when ctx
// is cancelled.  Use this for high-cardinality key spaces (e.g. per-user chat
// limiters) where lazy pruning alone could leave many stale map entries.
func NewWithPrune(ctx context.Context, pruneInterval time.Duration) *SlidingWindow {
	l := New()
	go l.runPrune(ctx, pruneInterval)
	return l
}

// Allow reports whether a new event for key should be permitted given that at
// most limit events are allowed within window.  It always returns a nil error;
// the error return exists so SlidingWindow satisfies common rate-limiter
// interfaces that require one.
func (l *SlidingWindow) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit <= 0 {
		return true, nil
	}
	now := time.Now()
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.byK == nil {
		l.byK = make(map[string][]time.Time)
	}
	prev := l.byK[key]
	kept := prev[:0]
	for _, ts := range prev {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= limit {
		l.byK[key] = kept
		return false, nil
	}
	l.byK[key] = append(kept, now)
	return true, nil
}

// runPrune ticks every interval and removes keys whose entire timestamp slice
// is older than window.  The window passed to prune is the longest window ever
// used by callers; using a value larger than necessary is safe — at worst a few
// extra bytes linger until the next tick.
func (l *SlidingWindow) runPrune(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Prune with a 1-hour horizon as a conservative upper bound.
			// Callers with shorter windows will see their stale keys cleaned
			// sooner via lazy trimming in Allow.
			l.prune(time.Hour)
		}
	}
}

// prune removes map entries whose every recorded timestamp is older than window.
func (l *SlidingWindow) prune(window time.Duration) {
	cutoff := time.Now().Add(-window)
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, ts := range l.byK {
		allStale := true
		for _, t := range ts {
			if t.After(cutoff) {
				allStale = false
				break
			}
		}
		if allStale {
			delete(l.byK, k)
		}
	}
}
