package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
)

// stubPaymentClient is a minimal gRPC client stub that always returns ok=true.
type stubPaymentClient struct {
	paymentv1.PaymentServiceClient
	called bool
}

func (s *stubPaymentClient) HandleWebhook(_ context.Context, _ *paymentv1.WebhookRequest, _ ...grpc.CallOption) (*paymentv1.WebhookResponse, error) {
	s.called = true
	return &paymentv1.WebhookResponse{Ok: true}, nil
}

// ctxWithIP injects a client IP into the context the same way the RealIP
// middleware does, so handlers under test see a realistic ctx value.
func ctxWithIP(r *http.Request, ip string) *http.Request {
	return middleware.WithClientIP(r, ip)
}

// makeHMAC computes the X-Signature header value for the given secret and body.
func makeHMAC(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhook_NoSecurity_Accepted(t *testing.T) {
	stub := &stubPaymentClient{}
	h := NewPaymentHandler(zerolog.Nop(), stub, WebhookSecurityConfig{})

	body := `{"event":"payment.succeeded","object":{"id":"pay_1","status":"succeeded"}}`
	req := httptest.NewRequest(http.MethodPost, "/payments/webhook", strings.NewReader(body))
	rr := httptest.NewRecorder()

	h.Webhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if !stub.called {
		t.Fatal("expected HandleWebhook to be called")
	}
}

func TestWebhook_HMAC(t *testing.T) {
	const secret = "supersecret"
	body := `{"event":"payment.succeeded","object":{"id":"pay_1","status":"succeeded"}}`

	tests := []struct {
		name       string
		sigHeader  string
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "valid signature",
			sigHeader:  makeHMAC(secret, body),
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "wrong signature",
			sigHeader:  "sha256=" + strings.Repeat("0", 64),
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
		{
			name:       "missing header",
			sigHeader:  "",
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
		{
			name:       "malformed prefix",
			sigHeader:  "md5=abc",
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
		{
			name:       "non-hex digest",
			sigHeader:  "sha256=zzzzzzzz",
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubPaymentClient{}
			sec := WebhookSecurityConfig{Secret: []byte(secret)}
			h := NewPaymentHandler(zerolog.Nop(), stub, sec)

			req := httptest.NewRequest(http.MethodPost, "/payments/webhook", strings.NewReader(body))
			if tc.sigHeader != "" {
				req.Header.Set("X-Signature", tc.sigHeader)
			}
			rr := httptest.NewRecorder()
			h.Webhook(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("want status %d, got %d", tc.wantStatus, rr.Code)
			}
			if stub.called != tc.wantCalled {
				t.Errorf("HandleWebhook called=%v, want %v", stub.called, tc.wantCalled)
			}
		})
	}
}

func TestWebhook_IPAllowlist(t *testing.T) {
	body := `{"event":"payment.succeeded","object":{"id":"pay_1","status":"succeeded"}}`

	allowlist := "185.71.76.0/27,77.75.156.11/32"
	sec := ParseWebhookSecurityConfig("", allowlist)

	tests := []struct {
		name       string
		clientIP   string
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "IP in CIDR",
			clientIP:   "185.71.76.5",
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "exact allowed IP",
			clientIP:   "77.75.156.11",
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "IP outside CIDR",
			clientIP:   "1.2.3.4",
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
		{
			name:       "empty IP (unparseable)",
			clientIP:   "",
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
		{
			name:       "border of CIDR (185.71.76.31 = last in /27)",
			clientIP:   "185.71.76.31",
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
		{
			name:       "one past CIDR boundary (185.71.76.32)",
			clientIP:   "185.71.76.32",
			wantStatus: http.StatusForbidden,
			wantCalled: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubPaymentClient{}
			h := NewPaymentHandler(zerolog.Nop(), stub, sec)

			req := httptest.NewRequest(http.MethodPost, "/payments/webhook", strings.NewReader(body))
			req = ctxWithIP(req, tc.clientIP)
			rr := httptest.NewRecorder()
			h.Webhook(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("clientIP=%q: want status %d, got %d", tc.clientIP, tc.wantStatus, rr.Code)
			}
			if stub.called != tc.wantCalled {
				t.Errorf("clientIP=%q: HandleWebhook called=%v, want %v", tc.clientIP, stub.called, tc.wantCalled)
			}
		})
	}
}

func TestWebhook_BothChecks_BothMustPass(t *testing.T) {
	const secret = "s3cr3t"
	body := `{"event":"payment.succeeded","object":{"id":"pay_1","status":"succeeded"}}`

	sec := ParseWebhookSecurityConfig(secret, "185.71.76.0/27")

	t.Run("valid IP and valid HMAC", func(t *testing.T) {
		stub := &stubPaymentClient{}
		h := NewPaymentHandler(zerolog.Nop(), stub, sec)

		req := httptest.NewRequest(http.MethodPost, "/payments/webhook", strings.NewReader(body))
		req = ctxWithIP(req, "185.71.76.10")
		req.Header.Set("X-Signature", makeHMAC(secret, body))
		rr := httptest.NewRecorder()
		h.Webhook(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("want 200, got %d", rr.Code)
		}
		if !stub.called {
			t.Error("expected HandleWebhook to be called")
		}
	})

	t.Run("valid IP but wrong HMAC", func(t *testing.T) {
		stub := &stubPaymentClient{}
		h := NewPaymentHandler(zerolog.Nop(), stub, sec)

		req := httptest.NewRequest(http.MethodPost, "/payments/webhook", strings.NewReader(body))
		req = ctxWithIP(req, "185.71.76.10")
		req.Header.Set("X-Signature", "sha256="+strings.Repeat("0", 64))
		rr := httptest.NewRecorder()
		h.Webhook(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("want 403, got %d", rr.Code)
		}
	})

	t.Run("wrong IP and valid HMAC", func(t *testing.T) {
		stub := &stubPaymentClient{}
		h := NewPaymentHandler(zerolog.Nop(), stub, sec)

		req := httptest.NewRequest(http.MethodPost, "/payments/webhook", strings.NewReader(body))
		req = ctxWithIP(req, "1.2.3.4")
		req.Header.Set("X-Signature", makeHMAC(secret, body))
		rr := httptest.NewRecorder()
		h.Webhook(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Errorf("want 403, got %d", rr.Code)
		}
	})
}

func TestParseWebhookSecurityConfig(t *testing.T) {
	t.Run("empty inputs", func(t *testing.T) {
		cfg := ParseWebhookSecurityConfig("", "")
		if len(cfg.Secret) != 0 {
			t.Error("expected nil secret")
		}
		if len(cfg.AllowedCIDRs) != 0 {
			t.Error("expected empty allowlist")
		}
	})

	t.Run("plain IP normalised to /32", func(t *testing.T) {
		cfg := ParseWebhookSecurityConfig("", "77.75.156.11")
		if len(cfg.AllowedCIDRs) != 1 {
			t.Fatalf("want 1 CIDR, got %d", len(cfg.AllowedCIDRs))
		}
		if !cfg.AllowedCIDRs[0].Contains(net.ParseIP("77.75.156.11")) {
			t.Error("expected /32 to contain the exact IP")
		}
	})

	t.Run("invalid entry skipped", func(t *testing.T) {
		cfg := ParseWebhookSecurityConfig("", "not-an-ip,185.71.76.0/27")
		if len(cfg.AllowedCIDRs) != 1 {
			t.Fatalf("want 1 CIDR (bad entry skipped), got %d", len(cfg.AllowedCIDRs))
		}
	})
}
