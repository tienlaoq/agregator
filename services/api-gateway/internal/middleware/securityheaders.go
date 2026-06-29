package middleware

import "net/http"

// SecurityHeaders sets baseline headers for every API response (defense in depth).
//
// The gateway serves JSON and uploaded media — never HTML documents — so the CSP
// locks everything down: default-src 'none' (nothing may be loaded by a response
// interpreted as a document), frame-ancestors 'none' (no embedding, the modern
// X-Frame-Options), base-uri 'none'. HSTS is intentionally not set here — it is
// owned by the edge proxy (Caddy), the single TLS-terminating layer.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}
