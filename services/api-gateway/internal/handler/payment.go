package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/rs/zerolog"
	paymentv1 "github.com/tienlao/agregator/gen/go/payment/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/httpx"
	"github.com/tienlao/agregator/services/api-gateway/internal/limits"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// WebhookSecurityConfig carries the two independent verification mechanisms
// for inbound payment webhooks.
//
// IP allowlist:
//
//	Acquiring gateways send notifications from a fixed published set of IPs.
//	When AllowedCIDRs is non-empty, any request whose source IP is not covered
//	by at least one entry is rejected with 403 before the body is read.
//	Configure with PAYMENT_WEBHOOK_IP_ALLOWLIST (comma-separated CIDRs or IPs).
//
// HMAC-SHA256 signature (new bank acquirer):
//
//	The acquirer signs each request body with a shared secret and includes the
//	result in the X-Signature header as "sha256=<lowercase hex>".
//	When Secret is non-empty, a missing or incorrect header causes a 403.
//	Configure with PAYMENT_WEBHOOK_SECRET.
//
// In local dev both fields may be empty — all requests are accepted, but a
// startup warning is emitted so the gap is visible in logs.
type WebhookSecurityConfig struct {
	// Secret is the raw HMAC-SHA256 key.  Empty disables HMAC verification.
	Secret []byte

	// AllowedCIDRs is the parsed allowlist.  Empty disables IP verification.
	AllowedCIDRs []*net.IPNet
}

// ParseWebhookSecurityConfig parses the two raw env-var strings into a typed config.
// rawSecret is the hex or plain secret from PAYMENT_WEBHOOK_SECRET.
// rawCIDRs is a comma-separated list of CIDRs / plain IPs from PAYMENT_WEBHOOK_IP_ALLOWLIST.
func ParseWebhookSecurityConfig(rawSecret, rawCIDRs string) WebhookSecurityConfig {
	cfg := WebhookSecurityConfig{}

	if s := strings.TrimSpace(rawSecret); s != "" {
		cfg.Secret = []byte(s)
	}

	for _, part := range strings.Split(rawCIDRs, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Accept plain IPs without a prefix — normalise to /32 or /128.
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
		cfg.AllowedCIDRs = append(cfg.AllowedCIDRs, ipNet)
	}

	return cfg
}

type PaymentHandler struct {
	client   paymentv1.PaymentServiceClient
	log      zerolog.Logger
	security WebhookSecurityConfig
}

func NewPaymentHandler(log zerolog.Logger, client paymentv1.PaymentServiceClient, sec WebhookSecurityConfig) *PaymentHandler {
	if len(sec.Secret) == 0 && len(sec.AllowedCIDRs) == 0 {
		log.Warn().Msg("payment webhook: neither PAYMENT_WEBHOOK_SECRET nor PAYMENT_WEBHOOK_IP_ALLOWLIST is configured — all callers accepted (dev mode only)")
	}
	return &PaymentHandler{client: client, log: log, security: sec}
}

// webhookBodyLog is the exhaustive list of fields that may appear in any log
// entry that touches a webhook request body or its derivatives.
//
// POLICY — webhook body MUST NEVER be logged, in whole or in part:
//
//   The raw body can contain card metadata, payer details, amounts, and other
//   PII fields that the payment provider may add in future API versions without notice.
//   Logging any fragment of it (even on error paths) risks writing PII to
//   stdout/files that feed aggregators such as Loki or CloudWatch Logs.
//
// Permitted in logs:
//   - body_bytes  — the byte length of the received body (no content)
//   - content_type — the Content-Type header (structural, never PII)
//   - client_ip   — already logged on rejection; stripped to /24 if desired
//   - object_id   — the provider payment UUID extracted after parse (non-sensitive)
//   - event       — the event type string, e.g. "payment.succeeded"
//   - object_status — the status string, e.g. "succeeded"
//
// NOT permitted (add to this list when new fields are considered):
//   - body / raw_body / payload — the full or partial request body
//   - amount / value / currency — financial data
//   - metadata.*               — may contain booking or user details
//   - payer / recipient         — direct PII
//   - description               — free-text, may contain user-provided content
//
// Reviewers: if you see any log statement in this file that emits a field not
// in the Permitted list, treat it as a PII leak and reject the PR.

func (h *PaymentHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	// Limit body to 256 KiB — sufficient for any payment-provider payload.
	// MaxBytesReader returns *http.MaxBytesError on overflow; treat that as a
	// client error (400) rather than a server-side read failure.
	//
	// NOTE: body is read here for HMAC verification only.  It must NOT be
	// passed to any logger — see webhookBodyLog policy above.
	r.Body = http.MaxBytesReader(w, r.Body, limits.PaymentWebhookMaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpx.WriteCatalog(w, apicatalog.GatewayRequestInvalidBody)
			return
		}
		httpx.WriteCatalog(w, apicatalog.GatewayRequestBodyReadFailed)
		return
	}

	// ── Layer 1: IP allowlist ─────────────────────────────────────────────
	// Checked against the resolved real client IP (set by the RealIP middleware
	// from a trusted X-Forwarded-For header or directly from RemoteAddr).
	if len(h.security.AllowedCIDRs) > 0 {
		clientIP := middleware.ClientIPFromCtx(r.Context())
		if !h.ipAllowed(clientIP) {
			h.log.Warn().
				Str("client_ip", clientIP).
				Msg("payment webhook rejected: source IP not in allowlist")
			httpx.WriteCatalog(w, apicatalog.GatewayPaymentWebhookForbidden)
			return
		}
	}

	// ── Layer 2: HMAC-SHA256 signature ────────────────────────────────────
	// Header format (same as GitHub webhooks): X-Signature: sha256=<hex>
	if len(h.security.Secret) > 0 {
		if !h.signatureValid(r.Header.Get("X-Signature"), body) {
			h.log.Warn().
				Str("client_ip", middleware.ClientIPFromCtx(r.Context())).
				Msg("payment webhook rejected: invalid or missing X-Signature")
			httpx.WriteCatalog(w, apicatalog.GatewayPaymentWebhookForbidden)
			return
		}
	}

	// Permitted fields only — see webhookBodyLog policy above.
	h.log.Info().
		Int("body_bytes", len(body)).
		Str("content_type", r.Header.Get("Content-Type")).
		Msg("payment webhook verified and accepted")

	resp, err := h.client.HandleWebhook(r.Context(), &paymentv1.WebhookRequest{
		RawBody: body,
	})
	if err != nil {
		h.log.Error().Err(err).Msg("forward webhook to payment-service")
		// Return 500 so the provider retries the notification.
		// The IP + HMAC checks above already confirmed the caller is legitimate;
		// a transient processing failure should not be silently swallowed.
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !resp.Ok {
		h.log.Error().Msg("payment-service returned ok=false for webhook")
		// Same reasoning: signal failure so the provider retries.
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ipAllowed returns true when clientIP string is covered by at least one entry
// in the configured allowlist.  An unparseable IP is never allowed.
func (h *PaymentHandler) ipAllowed(clientIP string) bool {
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return false
	}
	for _, cidr := range h.security.AllowedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// signatureValid verifies the X-Signature header against the request body.
// Expected format: "sha256=<lowercase hex of HMAC-SHA256(secret, body)>".
// Uses hmac.Equal for constant-time comparison to prevent timing attacks.
func (h *PaymentHandler) signatureValid(header string, body []byte) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	gotHex := header[len(prefix):]
	got, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, h.security.Secret)
	mac.Write(body)
	expected := mac.Sum(nil)

	return hmac.Equal(got, expected)
}
