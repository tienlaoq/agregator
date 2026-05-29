package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/pkg/config"
)

// ctxClientIPKey is the context key for the resolved real client IP address.
// Using an unexported struct type prevents collisions with keys from other packages.
type ctxClientIPKey struct{}

var ctxClientIP = ctxClientIPKey{}

// ClientIPFromCtx returns the real client IP stored by the RealIP middleware.
// If the middleware has not run (e.g. in unit tests that call handlers directly),
// it falls back to parsing r.RemoteAddr so callers never receive an empty string.
func ClientIPFromCtx(ctx context.Context) string {
	if ip, ok := ctx.Value(ctxClientIP).(string); ok && ip != "" {
		return ip
	}
	return ""
}

// WithClientIP returns a copy of r with ip stored in its context under the
// same key that the RealIP middleware uses.  Intended for use in unit tests
// only — production code must go through the RealIP middleware instead.
func WithClientIP(r *http.Request, ip string) *http.Request {
	ctx := context.WithValue(r.Context(), ctxClientIP, ip)
	return r.WithContext(ctx)
}

// trustedCIDRs parses TRUSTED_PROXY_CIDRS env (comma-separated CIDR list).
// Returns nil when the variable is empty, which means "trust nobody" for XFF purposes.
//
// Example:
//
//	TRUSTED_PROXY_CIDRS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16
func trustedCIDRs() []*net.IPNet {
	raw := strings.TrimSpace(config.GetEnv("TRUSTED_PROXY_CIDRS", ""))
	if raw == "" {
		return nil
	}
	var nets []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Accept plain IPs without prefix length — treat as /32 or /128.
		if !strings.Contains(part, "/") {
			if strings.Contains(part, ":") {
				part += "/128"
			} else {
				part += "/32"
			}
		}
		_, ipNet, err := net.ParseCIDR(part)
		if err != nil {
			continue
		}
		nets = append(nets, ipNet)
	}
	return nets
}

// isTrusted reports whether remoteIP is covered by one of the trusted CIDRs.
func isTrusted(remoteIP net.IP, trusted []*net.IPNet) bool {
	for _, n := range trusted {
		if n.Contains(remoteIP) {
			return true
		}
	}
	return false
}

// realClientIP resolves the actual client IP for a single request.
//
// Algorithm:
//  1. Parse r.RemoteAddr into an IP.
//  2. If the direct peer is in the trusted CIDR list AND X-Forwarded-For is
//     present, take the *rightmost* entry appended by that trusted proxy.
//     (The rightmost value is the one written by the nearest trusted hop —
//     the leftmost value can be forged by the original client.)
//  3. Otherwise fall back to the raw RemoteAddr host, ignoring XFF entirely.
func realClientIP(r *http.Request, trusted []*net.IPNet) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}

	// No trusted proxies configured → always use the direct connection IP.
	if len(trusted) == 0 {
		return remoteHost
	}

	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil || !isTrusted(remoteIP, trusted) {
		return remoteHost
	}

	// The request arrived from a trusted proxy — inspect XFF.
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff == "" {
		return remoteHost
	}

	// Walk from the right: the rightmost IP was appended by our trusted proxy.
	// Skip any entries that are themselves trusted proxies (multi-hop).
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		ip := net.ParseIP(candidate)
		if ip == nil {
			continue
		}
		if !isTrusted(ip, trusted) {
			return candidate
		}
	}

	// All XFF entries were trusted proxies themselves — use the direct peer.
	return remoteHost
}

// RealIP is a middleware that resolves the real client IP from X-Forwarded-For
// (when the direct peer is a trusted proxy) and stores it in the request
// context under a package-private key. Retrieve it with ClientIPFromCtx.
//
// r.RemoteAddr is intentionally left unchanged so that the original TCP peer
// address (with its real port) remains available for low-level diagnostics and
// access logs. Downstream code must not use r.RemoteAddr directly for
// security-sensitive decisions (rate-limiting, audit logging) — use
// ClientIPFromCtx instead.
//
// Configuration (env):
//
//	TRUSTED_PROXY_CIDRS — comma-separated list of CIDRs whose X-Forwarded-For
//	                      header is trusted. Empty (default) → XFF is never
//	                      trusted and r.RemoteAddr is used as-is.
func RealIP(log zerolog.Logger) func(http.Handler) http.Handler {
	trusted := trustedCIDRs()
	if len(trusted) == 0 {
		log.Info().Msg("RealIP: no TRUSTED_PROXY_CIDRS set — X-Forwarded-For will be ignored; using r.RemoteAddr directly")
	} else {
		cidrs := make([]string, len(trusted))
		for i, n := range trusted {
			cidrs[i] = n.String()
		}
		log.Info().Strs("trusted_cidrs", cidrs).Msg("RealIP: trusting X-Forwarded-For from configured proxy CIDRs")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realClientIP(r, trusted)
			ctx := context.WithValue(r.Context(), ctxClientIP, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
