package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/ratelimit"
)

// rateLimiter mirrors ratelimit.Limiter; the interface is kept local so callers
// can also pass Redis-backed or test-double limiters without a hard dependency.
type rateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// KeyMode controls how the rate-limit key is derived for a request.
type KeyMode uint8

const (
	// KeyModeIP keys on the real client IP stored in context by the RealIP
	// middleware. Suitable for public, unauthenticated endpoints.
	KeyModeIP KeyMode = iota

	// KeyModeUser keys on the authenticated user ID from context; falls back
	// to IP when the request is anonymous. Suitable for authenticated routes
	// where one account should not exhaust the shared IP budget.
	KeyModeUser
)

// RateLimitConfig defines the behaviour of a single RateLimit middleware instance.
type RateLimitConfig struct {
	// KeyPrefix distinguishes counters across endpoints (e.g. "rl:login:").
	KeyPrefix string

	// Max is the maximum number of requests allowed within Window.
	// 0 disables the limiter entirely.
	Max int

	// Window is the sliding-window duration for the counter.
	Window time.Duration

	// FailOpen controls behaviour when the backend (Redis) returns an error.
	// true  → allow the request and log a warning (default for most endpoints).
	// false → block the request (use for high-value endpoints like /auth/login).
	FailOpen bool

	// Mode selects the key derivation strategy.
	Mode KeyMode
}

// rateLimitKey returns the full Redis/in-memory key for the current request.
func rateLimitKey(r *http.Request, cfg RateLimitConfig) string {
	var id string
	if cfg.Mode == KeyModeUser {
		id = UserIDFromCtx(r.Context())
	}
	if id == "" {
		id = clientIP(r) // always available after RealIP middleware
	}
	return cfg.KeyPrefix + id
}

// RateLimit returns a per-endpoint rate-limiting middleware.
//
// When limiter is nil, an in-memory sliding-window store is used — correct for
// single-replica dev/test deployments. In production pass the Redis-backed
// limiter so limits are shared across replicas.
//
// Example (in main.go):
//
//	loginRL := middleware.RateLimit(log, redisLimiter, middleware.RateLimitConfig{
//	    KeyPrefix: "rl:login:", Max: 10, Window: 5*time.Minute, FailOpen: false,
//	})
//	api.With(loginRL).Post("/auth/login", authHandler.Login)
func RateLimit(log zerolog.Logger, limiter rateLimiter, cfg RateLimitConfig) func(http.Handler) http.Handler {
	if limiter == nil {
		limiter = ratelimit.New() // in-memory sliding window (single-replica / dev)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.Max <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			key := rateLimitKey(r, cfg)
			ok, err := limiter.Allow(r.Context(), key, cfg.Max, cfg.Window)
			if err != nil {
				if cfg.FailOpen {
					log.Warn().Err(err).Str("key", key).Str("prefix", cfg.KeyPrefix).
						Msg("rate limiter backend error, allowing request (fail-open)")
					next.ServeHTTP(w, r)
					return
				}
				log.Warn().Err(err).Str("key", key).Str("prefix", cfg.KeyPrefix).
					Msg("rate limiter backend error, blocking request (fail-closed)")
				apicatalog.GatewayRequestRateLimited.Write(w)
				return
			}
			if !ok {
				log.Warn().Str("key", key).Str("prefix", cfg.KeyPrefix).
					Msg("rate limit exceeded")
				apicatalog.GatewayRequestRateLimited.Write(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
