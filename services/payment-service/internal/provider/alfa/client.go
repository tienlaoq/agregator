// Package alfa implements provider.PaymentProvider against Alfa-Bank's
// internet-acquiring gateway, which runs on the RBS REST platform shared by
// several Russian banks. Single-stage charges are used: the payer confirms once
// and the full amount is captured onto the platform's single account (the escrow
// model) — there is no separate two-phase capture step.
//
// Endpoints (relative to the configured base URL):
//
//	register.do                — create a payment, returns formUrl to redirect to
//	refund.do                  — refund a deposited order
//	getOrderStatusExtended.do  — authoritative status pull (source of truth)
//
// Auth is a userName/password pair created in the Alfa merchant cabinet (the
// technical "…-api" user). Requests are form-urlencoded POST; replies are JSON.
//
// Base URLs (ALFA_GATEWAY_URL):
//
//	production: https://pay.alfabank.ru/payment/rest/
//	test:       https://alfa.rbsuat.com/payment/rest/
//
// Outbound partner payouts are NOT part of internet-acquiring — see CreatePayout.
package alfa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// currencyRUB is the ISO 4217 numeric code RBS expects for roubles.
const currencyRUB = "643"

// orderStatus values returned by getOrderStatusExtended.
const (
	rbsDeposited = 2 // fully paid
	rbsReversed  = 3 // authorization reversed / cancelled
	rbsRefunded  = 4 // refunded
	rbsDeclined  = 6 // payment declined
)

// Client is the Alfa-Bank RBS acquiring provider. Safe for concurrent use.
type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

// NewClient builds an Alfa provider. baseURL selects the gateway (prod vs the
// rbsuat test host); a trailing slash is normalised so callers may pass either.
func NewClient(username, password, baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/"
	return &Client{
		baseURL:  baseURL,
		username: username,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// IsMockMode reports false: this provider makes real network calls.
func (c *Client) IsMockMode() bool { return false }

// rbsResponse is the common envelope of every RBS REST reply. errorCode is
// decoded raw because RBS is inconsistent about quoting it ("0" vs 0).
type rbsResponse struct {
	ErrorCode    json.RawMessage `json:"errorCode"`
	ErrorMessage string          `json:"errorMessage"`
	OrderID      string          `json:"orderId"`
	FormURL      string          `json:"formUrl"`
	OrderStatus  *int            `json:"orderStatus"`
}

// code returns the normalised error code; "0" (or an absent field) means success.
func (r rbsResponse) code() string {
	s := strings.Trim(string(r.ErrorCode), `"`)
	if s == "" {
		return "0"
	}
	return s
}

func (r rbsResponse) ok() bool { return r.code() == "0" }

// call posts form params to an RBS method and decodes the JSON envelope.
// Network failures and 5xx replies are wrapped with provider.ErrTransient: the
// request may have reached the gateway, so the caller must reconcile, not roll
// back. 4xx and business rejections are returned as permanent errors.
func (c *Client) call(ctx context.Context, method string, params url.Values) (*rbsResponse, error) {
	params.Set("userName", c.username)
	params.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+method, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("alfa: build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alfa: %s request failed: %w: %w", method, err, provider.ErrTransient)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("alfa: read %s response: %w", method, err)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("alfa: %s upstream %d: %w", method, resp.StatusCode, provider.ErrTransient)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alfa: %s unexpected status %d", method, resp.StatusCode)
	}

	var out rbsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("alfa: decode %s response: %w", method, err)
	}
	return &out, nil
}

// CreatePayment registers a single-stage order and returns the gateway id plus
// the redirect URL the payer is sent to.
func (c *Client) CreatePayment(ctx context.Context, req provider.CreateRequest) (*provider.Result, error) {
	params := url.Values{}
	params.Set("orderNumber", deriveOrderNumber(req.IdempotencyKey))
	params.Set("amount", strconv.FormatInt(req.AmountKopecks, 10))
	params.Set("currency", currencyRUB)
	params.Set("returnUrl", req.ReturnURL)
	if req.Description != "" {
		params.Set("description", clip(req.Description, 512))
	}
	if jp := jsonParams(req); jp != "" {
		params.Set("jsonParams", jp)
	}

	resp, err := c.call(ctx, "register.do", params)
	if err != nil {
		return nil, err
	}
	if !resp.ok() {
		return nil, fmt.Errorf("alfa: register rejected (code %s): %s", resp.code(), resp.ErrorMessage)
	}
	if resp.OrderID == "" || resp.FormURL == "" {
		return nil, fmt.Errorf("alfa: register returned empty orderId/formUrl")
	}
	return &provider.Result{ProviderPaymentID: resp.OrderID, ConfirmationURL: resp.FormURL}, nil
}

// Capture is a no-op: register.do captures on the payer's confirmation, so there
// is no separate deposit step. ParseWebhook never sets RequiresCapture, so the
// usecase never calls this — it exists only to satisfy the interface.
func (c *Client) Capture(context.Context, string, string) error { return nil }

// Refund reverses a deposited order. The RBS refund is keyed by orderId+amount
// and is naturally idempotent, so idempotencyKey is unused.
func (c *Client) Refund(ctx context.Context, providerPaymentID string, amountKopecks int64, _ string) error {
	params := url.Values{}
	params.Set("orderId", providerPaymentID)
	params.Set("amount", strconv.FormatInt(amountKopecks, 10))

	resp, err := c.call(ctx, "refund.do", params)
	if err != nil {
		return err
	}
	if !resp.ok() {
		return fmt.Errorf("alfa: refund rejected (code %s): %s", resp.code(), resp.ErrorMessage)
	}
	return nil
}

// ParseWebhook resolves the order id from the callback and then pull-confirms the
// authoritative status via getOrderStatusExtended. The callback body itself is
// never trusted for state changes — only the pulled status is.
func (c *Client) ParseWebhook(ctx context.Context, rawBody []byte, _ http.Header) (*domain.WebhookEvent, error) {
	orderID := extractOrderID(rawBody)
	if orderID == "" {
		return nil, fmt.Errorf("alfa: webhook missing order id")
	}

	resp, err := c.call(ctx, "getOrderStatusExtended.do", url.Values{"orderId": {orderID}})
	if err != nil {
		return nil, err
	}
	if resp.OrderStatus == nil {
		return nil, fmt.Errorf("alfa: status pull returned no orderStatus (code %s): %s", resp.code(), resp.ErrorMessage)
	}

	st := *resp.OrderStatus
	return &domain.WebhookEvent{
		ProviderPaymentID: orderID,
		Status:            mapStatus(st),
		RawEvent:          "alfa.callback",
		RawProviderStatus: strconv.Itoa(st),
	}, nil
}

// mapStatus translates an RBS orderStatus into a domain status. Non-terminal or
// unknown values map to Pending so the usecase no-ops.
func mapStatus(s int) domain.PaymentStatus {
	switch s {
	case rbsDeposited:
		return domain.StatusSucceeded
	case rbsRefunded:
		return domain.StatusRefunded
	case rbsReversed, rbsDeclined:
		return domain.StatusCancelled
	default:
		return domain.StatusPending
	}
}

// ParsePayoutWebhook always returns (nil, nil): acquiring callbacks are never
// payout events, so the caller falls back to ParseWebhook.
func (c *Client) ParsePayoutWebhook(context.Context, []byte, http.Header) (*provider.PayoutWebhookEvent, error) {
	return nil, nil
}

// CreatePayout is not supported by Alfa internet-acquiring. Outbound partner
// payouts require Alfa's separate "Выплаты" / СБП B2C product, which has its own
// contract, endpoints, and signing. It fails loudly rather than fabricate a
// money-moving request that cannot be verified against the live gateway.
//
// ponytail: acquiring only. Wire a real Alfa payout call here once the СБП B2C
// API spec and credentials are available.
func (c *Client) CreatePayout(context.Context, provider.PayoutRequest) (*provider.PayoutResult, error) {
	return nil, errors.New("alfa: outbound payouts not wired — requires Alfa Выплаты/СБП B2C API (separate product)")
}

// VerifySignature returns nil. Alfa RBS signs callbacks with a checksum carried
// in the callback's own query parameters, which the api-gateway does not forward
// to this layer (only the raw body reaches ParseWebhook). Authenticity is
// therefore enforced at the gateway via the published Alfa callback IP allowlist
// (PAYMENT_WEBHOOK_IP_ALLOWLIST), and every event is independently pull-confirmed
// against getOrderStatusExtended — a forged body cannot move a payment to
// "succeeded" unless the gateway itself reports it deposited.
func (c *Client) VerifySignature(context.Context, []byte, http.Header) error { return nil }

// deriveOrderNumber turns our opaque idempotency key into a stable, gateway-safe
// order number. RBS caps orderNumber length and rejects duplicates, so a
// deterministic 30-char hex digest gives length safety and idempotency (a retry
// with the same key produces the same number) without exposing the internal key.
//
// ponytail: RBS treats a duplicate orderNumber as a hard rejection, not a silent
// idempotent replay — the usecase's stored payment row is what prevents a second
// register call. Add a pull-then-register reconcile path if transient-retry
// duplicates ever surface in practice.
func deriveOrderNumber(idempotencyKey string) string {
	sum := sha256.Sum256([]byte(idempotencyKey))
	return hex.EncodeToString(sum[:])[:30]
}

// jsonParams packs traceability fields into the RBS jsonParams field. Returns ""
// when there is nothing to carry.
func jsonParams(req provider.CreateRequest) string {
	m := map[string]string{}
	if req.BookingID != "" {
		m["bookingId"] = req.BookingID
	}
	if req.CounterpartyID != "" {
		m["counterpartyId"] = req.CounterpartyID
	}
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// extractOrderID pulls the gateway order id (mdOrder / orderId) from a callback
// body, accepting both the native RBS form encoding and a JSON re-wrap.
func extractOrderID(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var j struct {
			OrderID string `json:"orderId"`
			MdOrder string `json:"mdOrder"`
		}
		if err := json.Unmarshal(trimmed, &j); err != nil {
			return ""
		}
		return firstNonEmpty(j.OrderID, j.MdOrder)
	}
	v, err := url.ParseQuery(string(trimmed))
	if err != nil {
		return ""
	}
	return firstNonEmpty(v.Get("mdOrder"), v.Get("orderId"))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// clip trims s to at most n bytes on a valid boundary for short ASCII/UTF-8
// descriptions; RBS rejects over-long fields.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
