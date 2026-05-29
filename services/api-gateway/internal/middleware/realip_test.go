package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

// helpers ---------------------------------------------------------------

func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func trustedNets(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, mustParseCIDR(c))
	}
	return out
}

// realClientIP unit tests -----------------------------------------------

func TestRealClientIP_noTrustedProxies_usesRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "1.2.3.4:5678"
	r.Header.Set("X-Forwarded-For", "9.9.9.9")

	got := realClientIP(r, nil)
	assert.Equal(t, "1.2.3.4", got, "XFF must be ignored when no trusted proxies configured")
}

func TestRealClientIP_untrustedPeer_ignoresXFF(t *testing.T) {
	trusted := trustedNets("10.0.0.0/8")

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "1.2.3.4:5678" // NOT in 10.0.0.0/8
	r.Header.Set("X-Forwarded-For", "9.9.9.9")

	got := realClientIP(r, trusted)
	assert.Equal(t, "1.2.3.4", got, "XFF must be ignored when direct peer is not a trusted proxy")
}

func TestRealClientIP_trustedPeer_usesRightmostXFF(t *testing.T) {
	trusted := trustedNets("10.0.0.0/8")

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234" // trusted proxy
	// Left entry could be forged by client; rightmost is written by our proxy.
	r.Header.Set("X-Forwarded-For", "evil.client.ip, 203.0.113.5")

	got := realClientIP(r, trusted)
	assert.Equal(t, "203.0.113.5", got)
}

func TestRealClientIP_singleXFFEntry(t *testing.T) {
	trusted := trustedNets("172.16.0.0/12")

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "172.16.0.10:80"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")

	got := realClientIP(r, trusted)
	assert.Equal(t, "198.51.100.7", got)
}

func TestRealClientIP_multiHopTrustedChain(t *testing.T) {
	// Two trusted proxies in chain: client → proxy1 (10.0.0.1) → proxy2 (10.0.0.2) → gateway.
	// XFF: "client, proxy1" — rightmost non-trusted is "client".
	trusted := trustedNets("10.0.0.0/8")

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.2:443"
	r.Header.Set("X-Forwarded-For", "203.0.113.99, 10.0.0.1")

	got := realClientIP(r, trusted)
	assert.Equal(t, "203.0.113.99", got)
}

func TestRealClientIP_allXFFEntriesTrusted_fallsBackToRemote(t *testing.T) {
	trusted := trustedNets("10.0.0.0/8")

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.5:443"
	r.Header.Set("X-Forwarded-For", "10.0.0.3, 10.0.0.4") // all trusted

	got := realClientIP(r, trusted)
	assert.Equal(t, "10.0.0.5", got, "should fall back to direct peer when all XFF entries are trusted")
}

func TestRealClientIP_noXFF_trustedPeer(t *testing.T) {
	trusted := trustedNets("10.0.0.0/8")

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:9999"
	// No XFF header.

	got := realClientIP(r, trusted)
	assert.Equal(t, "10.0.0.1", got)
}

// RealIP middleware integration tests -----------------------------------

func TestRealIP_middleware_storesIPInContext(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")

	var capturedIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = ClientIPFromCtx(r.Context())
	})

	h := RealIP(zerolog.Nop())(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.42")

	h.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "203.0.113.42", capturedIP)
}

func TestRealIP_middleware_preservesOriginalRemoteAddr(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")

	var capturedRemoteAddr string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRemoteAddr = r.RemoteAddr
	})

	h := RealIP(zerolog.Nop())(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.42")

	h.ServeHTTP(httptest.NewRecorder(), req)

	// r.RemoteAddr must be unchanged — the real IP lives in context only.
	assert.Equal(t, "10.0.0.1:12345", capturedRemoteAddr,
		"RealIP must not mutate r.RemoteAddr; use ClientIPFromCtx instead")
}

func TestRealIP_middleware_noConfig_ignoresXFF(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "")

	var capturedIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = ClientIPFromCtx(r.Context())
	})

	h := RealIP(zerolog.Nop())(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "5.6.7.8:11111"
	req.Header.Set("X-Forwarded-For", "attacker.ip.address")

	h.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "5.6.7.8", capturedIP, "XFF must be ignored without TRUSTED_PROXY_CIDRS")
}

func TestRealIP_middleware_ipv6(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "::1/128")

	var capturedIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = ClientIPFromCtx(r.Context())
	})

	h := RealIP(zerolog.Nop())(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:9000"
	req.Header.Set("X-Forwarded-For", "2001:db8::1")

	h.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, "2001:db8::1", capturedIP)
}
