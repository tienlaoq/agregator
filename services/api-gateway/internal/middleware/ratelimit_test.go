package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tienlao/agregator/services/api-gateway/internal/ratelimit"
)

// stubRateLimiter is a test double that counts Allow calls and returns
// configurable results.
type stubRateLimiter struct {
	calls   []string
	allowFn func(key string) (bool, error)
}

func (s *stubRateLimiter) Allow(_ context.Context, key string, _ int, _ time.Duration) (bool, error) {
	s.calls = append(s.calls, key)
	if s.allowFn != nil {
		return s.allowFn(key)
	}
	return true, nil
}

func newReq(remoteAddr string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = remoteAddr + ":1234"
	return r
}

// --- basic allow / block ---

func TestRateLimit_allowsWhenUnderMax(t *testing.T) {
	stub := &stubRateLimiter{allowFn: func(_ string) (bool, error) { return true, nil }}
	cfg := RateLimitConfig{KeyPrefix: "rl:test:", Max: 5, Window: time.Minute, FailOpen: true}
	h := RateLimit(zerolog.Nop(), stub, cfg)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimit_blocksWhenLimitExceeded(t *testing.T) {
	stub := &stubRateLimiter{allowFn: func(_ string) (bool, error) { return false, nil }}
	cfg := RateLimitConfig{KeyPrefix: "rl:test:", Max: 1, Window: time.Minute}
	h := RateLimit(zerolog.Nop(), stub, cfg)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimit_disabledWhenMaxZero(t *testing.T) {
	stub := &stubRateLimiter{}
	cfg := RateLimitConfig{KeyPrefix: "rl:test:", Max: 0, Window: time.Minute}
	h := RateLimit(zerolog.Nop(), stub, cfg)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4"))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, stub.calls, "limiter must not be called when Max=0")
}

// --- fail-open / fail-closed ---

func TestRateLimit_failOpen_allowsOnError(t *testing.T) {
	stub := &stubRateLimiter{allowFn: func(_ string) (bool, error) {
		return false, errors.New("redis down")
	}}
	cfg := RateLimitConfig{KeyPrefix: "rl:test:", Max: 5, Window: time.Minute, FailOpen: true}
	h := RateLimit(zerolog.Nop(), stub, cfg)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimit_failClosed_blocksOnError(t *testing.T) {
	stub := &stubRateLimiter{allowFn: func(_ string) (bool, error) {
		return false, errors.New("redis down")
	}}
	cfg := RateLimitConfig{KeyPrefix: "rl:test:", Max: 5, Window: time.Minute, FailOpen: false}
	h := RateLimit(zerolog.Nop(), stub, cfg)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("1.2.3.4"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

// --- key modes ---

func TestRateLimit_keyModeIP_usesRemoteAddr(t *testing.T) {
	stub := &stubRateLimiter{allowFn: func(_ string) (bool, error) { return true, nil }}
	cfg := RateLimitConfig{KeyPrefix: "rl:ip:", Max: 10, Window: time.Minute, Mode: KeyModeIP}
	h := RateLimit(zerolog.Nop(), stub, cfg)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("5.6.7.8"))
	require.Len(t, stub.calls, 1)
	assert.Equal(t, "rl:ip:5.6.7.8", stub.calls[0])
}

func TestRateLimit_keyModeUser_usesUserIDWhenPresent(t *testing.T) {
	stub := &stubRateLimiter{allowFn: func(_ string) (bool, error) { return true, nil }}
	cfg := RateLimitConfig{KeyPrefix: "rl:user:", Max: 10, Window: time.Minute, Mode: KeyModeUser}
	h := RateLimit(zerolog.Nop(), stub, cfg)(okHandler())

	req := newReq("5.6.7.8")
	req = req.WithContext(WithUserID(req.Context(), "user-abc"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Len(t, stub.calls, 1)
	assert.Equal(t, "rl:user:user-abc", stub.calls[0])
}

func TestRateLimit_keyModeUser_fallsBackToIPWhenAnonymous(t *testing.T) {
	stub := &stubRateLimiter{allowFn: func(_ string) (bool, error) { return true, nil }}
	cfg := RateLimitConfig{KeyPrefix: "rl:user:", Max: 10, Window: time.Minute, Mode: KeyModeUser}
	h := RateLimit(zerolog.Nop(), stub, cfg)(okHandler())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("9.9.9.9"))
	require.Len(t, stub.calls, 1)
	assert.Equal(t, "rl:user:9.9.9.9", stub.calls[0])
}

// --- in-memory fallback (nil limiter) ---

func TestRateLimit_nilLimiter_usesInMemory(t *testing.T) {
	cfg := RateLimitConfig{KeyPrefix: "rl:mem:", Max: 2, Window: time.Hour}
	h := RateLimit(zerolog.Nop(), nil, cfg)(okHandler())

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newReq("10.0.0.1"))
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should be allowed", i+1)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("10.0.0.1"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "3rd request should be blocked")
}

// --- key prefix isolation ---

func TestRateLimit_differentPrefixes_independentCounters(t *testing.T) {
	// Two middleware instances with different prefixes share the same in-memory store
	// but must NOT share counters.
	cfg1 := RateLimitConfig{KeyPrefix: "rl:a:", Max: 1, Window: time.Hour}
	cfg2 := RateLimitConfig{KeyPrefix: "rl:b:", Max: 1, Window: time.Hour}

	// Shared in-memory limiter to verify prefix isolation.
	mem := ratelimit.New()
	h1 := RateLimit(zerolog.Nop(), mem, cfg1)(okHandler())
	h2 := RateLimit(zerolog.Nop(), mem, cfg2)(okHandler())

	// Exhaust cfg1 limit.
	h1.ServeHTTP(httptest.NewRecorder(), newReq("1.1.1.1"))
	rec := httptest.NewRecorder()
	h1.ServeHTTP(rec, newReq("1.1.1.1"))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "cfg1 should be limited")

	// cfg2 counter is independent — first request must pass.
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, newReq("1.1.1.1"))
	assert.Equal(t, http.StatusOK, rec2.Code, "cfg2 must have its own counter")
}

// helpers

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
