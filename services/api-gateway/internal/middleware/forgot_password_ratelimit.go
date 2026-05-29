package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/pkg/config"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/ratelimit"
)

type forgotPasswordLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// clientIP returns the resolved real client IP for rate-limiting and audit
// purposes. It prefers the value stored by the RealIP middleware in context
// (ClientIPFromCtx); if the middleware has not run (e.g. in tests that call
// handlers directly) it falls back to parsing r.RemoteAddr.
func clientIP(r *http.Request) string {
	if ip := ClientIPFromCtx(r.Context()); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ForgotPasswordRateLimit returns middleware that returns 429 when the same IP exceeds the limit.
// FORGOT_PASSWORD_RATE_LIMIT_MAX (default 10), 0 = disabled.
// FORGOT_PASSWORD_RATE_LIMIT_WINDOW (default 15m).
// FORGOT_PASSWORD_RATE_LIMIT_KEY_PREFIX (default auth:forgot:).
// FORGOT_PASSWORD_RATE_LIMIT_FAIL_OPEN (default true): if limiter backend errors, allow request.
func ForgotPasswordRateLimit(log zerolog.Logger, limiter forgotPasswordLimiter) func(http.Handler) http.Handler {
	if limiter == nil {
		limiter = ratelimit.New()
	}
	max := config.GetEnvInt("FORGOT_PASSWORD_RATE_LIMIT_MAX", 10)
	winStr := strings.TrimSpace(config.GetEnv("FORGOT_PASSWORD_RATE_LIMIT_WINDOW", "15m"))
	keyPrefix := strings.TrimSpace(config.GetEnv("FORGOT_PASSWORD_RATE_LIMIT_KEY_PREFIX", "auth:forgot:"))
	if keyPrefix == "" {
		keyPrefix = "auth:forgot:"
	}
	failOpen := config.GetEnvBool("FORGOT_PASSWORD_RATE_LIMIT_FAIL_OPEN", true)
	window, err := time.ParseDuration(winStr)
	if err != nil || window <= 0 {
		window = 15 * time.Minute
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if max <= 0 {
				next.ServeHTTP(w, r)
				return
			}
			ip := clientIP(r)
			ok, err := limiter.Allow(r.Context(), keyPrefix+ip, max, window)
			if err != nil {
				if failOpen {
					log.Warn().Err(err).Str("client_ip", ip).Msg("forgot-password rate limiter unavailable, allowing request")
					next.ServeHTTP(w, r)
					return
				}
				log.Warn().Err(err).Str("client_ip", ip).Msg("forgot-password rate limiter unavailable, blocking request")
				apicatalog.GatewayRequestRateLimited.Write(w)
				return
			}
			if !ok {
				log.Warn().Str("client_ip", ip).Msg("forgot-password rate limit exceeded")
				apicatalog.GatewayRequestRateLimited.Write(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
