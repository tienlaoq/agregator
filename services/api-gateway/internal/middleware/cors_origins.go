package middleware

import (
	"net/url"
	"os"
	"strings"
)

// IsLocalLoopbackOrigin is true for browser Origin pointing at localhost / 127.0.0.1 / ::1 (any port).
// Next.js often moves to :3001+ when :3000 is busy; WebSocket CheckOrigin must not depend on a single port.
func IsLocalLoopbackOrigin(origin string) bool {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Host == "" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// CORSAllowedOrigins returns explicit origins for credentialed browser requests (must not use "*").
// Set CORS_ALLOWED_ORIGINS to a comma-separated list (e.g. https://app.example.com,https://www.example.com).
// If empty, uses FRONTEND_URL when set, otherwise local dev defaults.
func CORSAllowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if raw != "" {
		var out []string
		for _, p := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if front := strings.TrimSpace(os.Getenv("FRONTEND_URL")); front != "" {
		return []string{front}
	}
	return []string{
		"http://localhost:3000", "http://127.0.0.1:3000",
		"http://localhost:3001", "http://127.0.0.1:3001",
	}
}
