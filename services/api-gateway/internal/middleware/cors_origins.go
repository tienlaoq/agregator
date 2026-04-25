package middleware

import (
	"os"
	"strings"
)

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
	return []string{"http://localhost:3000", "http://127.0.0.1:3000"}
}
