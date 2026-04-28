package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/pkg/config"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
)

// forgotPasswordHits limits POST /auth/forgot-password per client IP.
// In-memory only: suitable for single replica / dev. Behind several gateway replicas use a shared store (e.g. Redis).
type forgotPasswordHits struct {
	mu   sync.Mutex
	byIP map[string][]time.Time
}

func (h *forgotPasswordHits) allow(ip string, max int, window time.Duration) bool {
	if max <= 0 {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-window)

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.byIP == nil {
		h.byIP = make(map[string][]time.Time)
	}
	prev := h.byIP[ip]
	kept := prev[:0]
	for _, ts := range prev {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= max {
		h.byIP[ip] = kept
		return false
	}
	kept = append(kept, now)
	h.byIP[ip] = kept
	return true
}

func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = strings.TrimSpace(xff[:i])
		}
		return xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ForgotPasswordRateLimit returns middleware that returns 429 when the same IP exceeds the limit.
// FORGOT_PASSWORD_RATE_LIMIT_MAX (default 10), 0 = disabled. FORGOT_PASSWORD_RATE_LIMIT_WINDOW (default 15m).
func ForgotPasswordRateLimit(log zerolog.Logger) func(http.Handler) http.Handler {
	h := &forgotPasswordHits{}
	max := config.GetEnvInt("FORGOT_PASSWORD_RATE_LIMIT_MAX", 10)
	winStr := strings.TrimSpace(config.GetEnv("FORGOT_PASSWORD_RATE_LIMIT_WINDOW", "15m"))
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
			if !h.allow(ip, max, window) {
				log.Warn().Str("client_ip", ip).Msg("forgot-password rate limit exceeded")
				apicatalog.GatewayRequestRateLimited.Write(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
