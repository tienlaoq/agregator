// Package provider defines the PaymentProvider interface that decouples the
// payment usecase from any specific payment gateway.  All gateway-specific wire
// formats, status strings, and two-phase capture mechanics are encapsulated
// behind this interface; the usecase only sees provider-agnostic types.
//
// To add a new gateway (Tinkoff, Sber, Alfa):
//  1. Create a sub-package (e.g. provider/tbank/) that implements PaymentProvider.
//  2. Wire it in cmd/main.go — no usecase changes required.
package provider

import (
	"context"
	"errors"
	"net/http"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

// ErrTransient marks a provider error as retryable: the request may or may not
// have reached the provider (network timeout, 5xx, TCP reset). The caller must
// NOT compensate (refund/reverse) on this error — the provider may have
// accepted the request and the operation must be reconciled, not rolled back.
//
// Wrap network errors and 5xx responses as: fmt.Errorf("...: %w", ErrTransient).
// Errors NOT wrapped with this sentinel are treated as permanent (typically 4xx
// or business-rule rejections) and are safe to compensate immediately.
//
// Used by the payout flow to prevent double-payouts when a CreatePayout call
// times out after the provider has already issued the transfer.
var ErrTransient = errors.New("provider: transient error")

// CreateRequest is the provider-agnostic input for creating a new payment.
//
// With the escrow model the gateway charges the full amount onto our single
// platform account; no split/transfers are issued at the provider. Payouts
// to partners are made out-of-band via the CreatePayout flow below.
type CreateRequest struct {
	// AmountKopecks is the gross amount to charge (in kopecks).
	AmountKopecks int64
	// Description is the human-readable order description shown to the payer.
	Description string
	// ReturnURL is the redirect target after the payer completes the flow.
	ReturnURL string
	// IdempotencyKey is a stable, opaque string used to de-dup requests at the
	// provider level.  The provider implementation derives its own wire key from
	// this value (e.g. UUID v5) so our internal keys are never exposed.
	IdempotencyKey string
	// CounterpartyID is an opaque reference carried in provider metadata.
	CounterpartyID string
	// BookingID is carried in provider metadata for traceability.
	BookingID string
}

// Result is the provider-agnostic outcome of a successful CreatePayment call.
type Result struct {
	// ProviderPaymentID is the gateway's opaque payment identifier.
	ProviderPaymentID string
	// ConfirmationURL is the redirect URL the payer should be sent to.
	ConfirmationURL string
}

// PayoutDestinationKind selects how a payout is routed at the provider.
type PayoutDestinationKind string

const (
	PayoutDestCard        PayoutDestinationKind = "card"
	PayoutDestBankAccount PayoutDestinationKind = "bank_account"
	PayoutDestSBP         PayoutDestinationKind = "sbp"
)

// PayoutRequest is the provider-agnostic input for issuing an outbound
// payment to a partner. The destination shape is selected by Kind; only
// the fields matching that kind are read.
type PayoutRequest struct {
	AmountKopecks int64
	Currency      string // ISO 4217; defaults to RUB when empty
	Description   string
	// IdempotencyKey must be stable across retries so the provider collapses
	// duplicate requests to one payout.
	IdempotencyKey string

	Kind PayoutDestinationKind

	// Card / SBP rails
	ProviderToken string // opaque provider-issued payout token for the card
	SBPPhone      string
	SBPBankID     string

	// Bank-account rail
	BankBIC       string
	BankAccount   string
	RecipientINN  string
	RecipientKPP  string
	RecipientName string

	// PartnerType / PartnerID flow through to provider metadata for traceability.
	PartnerType string
	PartnerID   string
}

// PayoutStatus is the provider-agnostic status returned by CreatePayout /
// observed in a payout webhook. The usecase maps it to domain.PayoutStatus.
type PayoutStatus string

const (
	// PayoutStatusPending — provider accepted the request, awaiting async outcome.
	PayoutStatusPending PayoutStatus = "pending"
	// PayoutStatusSucceeded — terminal: funds delivered to the destination.
	PayoutStatusSucceeded PayoutStatus = "succeeded"
	// PayoutStatusFailed — terminal: provider rejected or the destination refused.
	PayoutStatusFailed PayoutStatus = "failed"
)

// PayoutResult is the provider-agnostic outcome of CreatePayout.
type PayoutResult struct {
	ProviderPayoutID string
	Status           PayoutStatus
	FailureReason    string // set only when Status == PayoutStatusFailed
}

// PayoutWebhookEvent is the parsed and pull-confirmed outcome of an inbound
// payout notification.
type PayoutWebhookEvent struct {
	ProviderPayoutID string
	Status           PayoutStatus
	FailureReason    string
	RawEvent         string
}

// PaymentProvider is the single abstraction over external payment gateways.
// The usecase depends exclusively on this interface; no gateway type leaks into
// the business logic layer.
//
// All methods accept a context.Context as their first argument.  Implementations
// must propagate it to every outbound HTTP request so that upstream gRPC
// deadlines and graceful-shutdown cancellations are honoured.  Using
// context.Background() internally (as was done before) causes retry loops to
// outlive the caller and prevents clean shutdown.
//
// All implementations must be safe for concurrent use.
type PaymentProvider interface {
	// CreatePayment initiates a new payment at the gateway and returns the
	// provider reference and redirect URL. The full amount lands on the
	// platform's single account; partner payouts are issued separately
	// via CreatePayout.
	CreatePayment(ctx context.Context, req CreateRequest) (*Result, error)

	// Capture confirms a previously authorized payment (two-phase capture).
	// idempotencyKey must be stable across retries for the same capture.
	Capture(ctx context.Context, providerPaymentID, idempotencyKey string) error

	// Refund issues a full refund for an already-captured payment.
	// idempotencyKey must be stable across retries to prevent double-refunds.
	Refund(ctx context.Context, providerPaymentID string, amountKopecks int64, idempotencyKey string) error

	// CreatePayout issues an outbound payout to a partner via the destination
	// described by req.Kind.  Returns the provider's opaque payout id and the
	// initial status (typically Pending — terminal outcome arrives via webhook).
	//
	// idempotencyKey must be stable across retries: the provider deduplicates
	// requests with the same key, so a retry after a network blip never
	// double-charges the platform.
	CreatePayout(ctx context.Context, req PayoutRequest) (*PayoutResult, error)

	// ParseWebhook parses the raw notification body, applies pull-confirm if
	// required, and returns a provider-agnostic WebhookEvent.  All
	// provider-specific status strings and capture mechanics are hidden here.
	//
	// The rawBody slice must not be retained after ParseWebhook returns.
	ParseWebhook(ctx context.Context, rawBody []byte, headers http.Header) (*domain.WebhookEvent, error)

	// ParsePayoutWebhook parses an inbound payout notification.  Returns nil
	// event when the notification is not a payout (e.g. a payment webhook on
	// the same endpoint) so the caller can fall back to ParseWebhook.
	ParsePayoutWebhook(ctx context.Context, rawBody []byte, headers http.Header) (*PayoutWebhookEvent, error)

	// VerifySignature validates the authenticity of an inbound webhook
	// notification without parsing its content.  Returns nil if the signature
	// is valid; returns an error otherwise.  Called before ParseWebhook so that
	// tampered bodies are rejected before any pull-confirm round-trip.
	//
	// Implementations that rely on IP allowlisting rather than HMAC may return
	// nil unconditionally, but must document this clearly.
	VerifySignature(ctx context.Context, rawBody []byte, headers http.Header) error

	// IsMockMode reports whether the provider is running without real network
	// calls (e.g. in dev/CI).  Used by the usecase to relax validation rules
	// that require live credentials.
	IsMockMode() bool
}
