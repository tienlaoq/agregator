// Package mock implements provider.PaymentProvider without any external gateway.
// It is the default provider while no real bank acquiring is wired up: every
// method succeeds locally, returning synthetic identifiers and redirect URLs so
// the booking → payment → payout flow can be exercised end-to-end in dev and CI.
//
// It is NOT a real gateway and must never run in production — config.Validate
// rejects PAYMENT_PROVIDER=mock when ENV=production so a deploy can never charge
// or pay out through this stand-in. When the acquiring bank is chosen, add a
// real implementation under internal/provider/<bank>/ and select it via
// PAYMENT_PROVIDER; no usecase or domain changes are required.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// Client is the no-network payment provider. The zero value is ready to use;
// NewClient is provided for symmetry with real providers. Safe for concurrent use.
type Client struct{}

// NewClient returns a mock payment provider. It takes no credentials because it
// never talks to an external gateway.
func NewClient() *Client { return &Client{} }

// IsMockMode always reports true: this provider performs no real network calls.
func (c *Client) IsMockMode() bool { return true }

// CreatePayment returns a synthetic payment id and a fake confirmation URL.
func (c *Client) CreatePayment(_ context.Context, _ provider.CreateRequest) (*provider.Result, error) {
	id := uuid.New().String()
	return &provider.Result{
		ProviderPaymentID: id,
		ConfirmationURL:   fmt.Sprintf("https://mock.local/pay/%s", id),
	}, nil
}

// Capture is a no-op: the mock provider has no two-phase capture.
func (c *Client) Capture(_ context.Context, _, _ string) error { return nil }

// Refund is a no-op: there is no external charge to reverse.
func (c *Client) Refund(_ context.Context, _ string, _ int64, _ string) error { return nil }

// CreatePayout returns a synthetic payout that succeeds immediately, mirroring
// the charge flow so payout reconciliation can be exercised without a webhook
// round-trip during local development.
func (c *Client) CreatePayout(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
	return &provider.PayoutResult{
		ProviderPayoutID: uuid.New().String(),
		Status:           provider.PayoutStatusSucceeded,
	}, nil
}

// VerifySignature accepts every notification: the mock provider issues none, so
// there is nothing to authenticate.
func (c *Client) VerifySignature(_ context.Context, _ []byte, _ http.Header) error { return nil }

// webhookPayload is the minimal, provider-agnostic shape used by the mock so a
// developer can POST a hand-crafted notification to the webhook endpoint.
type webhookPayload struct {
	Event  string `json:"event"`
	Object struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"object"`
}

// statusToDomain maps the synthetic payload status to a domain status.
// Unknown values are treated as non-terminal (the usecase no-ops).
var statusToDomain = map[string]domain.PaymentStatus{
	"succeeded": domain.StatusSucceeded,
	"canceled":  domain.StatusCancelled,
	"cancelled": domain.StatusCancelled,
}

// ParseWebhook parses a synthetic notification. No pull-confirm is performed:
// the payload status is taken as authoritative because the mock provider is the
// sole source of these events. A "waiting_for_capture" status surfaces the
// capture flag so the capture path stays exercisable locally.
func (c *Client) ParseWebhook(_ context.Context, rawBody []byte, _ http.Header) (*domain.WebhookEvent, error) {
	var payload webhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, fmt.Errorf("parse webhook body: %w", err)
	}
	providerID := payload.Object.ID
	if providerID == "" {
		return nil, fmt.Errorf("webhook: missing payment id in payload")
	}

	if payload.Object.Status == "waiting_for_capture" {
		return &domain.WebhookEvent{
			ProviderPaymentID: providerID,
			Status:            domain.StatusPending,
			RequiresCapture:   true,
			CaptureKey:        providerID + "-capture",
			RawEvent:          payload.Event,
			RawProviderStatus: payload.Object.Status,
		}, nil
	}

	status, ok := statusToDomain[payload.Object.Status]
	if !ok {
		status = domain.StatusPending
	}
	return &domain.WebhookEvent{
		ProviderPaymentID: providerID,
		Status:            status,
		RawEvent:          payload.Event,
		RawProviderStatus: payload.Object.Status,
	}, nil
}

// payoutStatusToDomain maps the synthetic payout payload status to a
// provider-agnostic status.
var payoutStatusToDomain = map[string]provider.PayoutStatus{
	"pending":   provider.PayoutStatusPending,
	"succeeded": provider.PayoutStatusSucceeded,
	"canceled":  provider.PayoutStatusFailed,
	"cancelled": provider.PayoutStatusFailed,
}

// ParsePayoutWebhook parses a synthetic payout notification. It returns
// (nil, nil) when the event is not a payout event so the caller falls back to
// ParseWebhook, matching the real-provider contract.
func (c *Client) ParsePayoutWebhook(_ context.Context, rawBody []byte, _ http.Header) (*provider.PayoutWebhookEvent, error) {
	var payload webhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, fmt.Errorf("parse payout webhook body: %w", err)
	}
	if !strings.HasPrefix(payload.Event, "payout.") {
		return nil, nil // not a payout event; caller falls back to payment parsing
	}
	if payload.Object.ID == "" {
		return nil, fmt.Errorf("payout webhook: missing id in payload")
	}
	status, ok := payoutStatusToDomain[payload.Object.Status]
	if !ok {
		status = provider.PayoutStatusPending
	}
	return &provider.PayoutWebhookEvent{
		ProviderPayoutID: payload.Object.ID,
		Status:           status,
		RawEvent:         payload.Event,
	}, nil
}
