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
	"net/http"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

// CreateRequest is the provider-agnostic input for creating a new payment.
// Whether the payment is a simple charge or a marketplace split is determined
// by the presence of SellerAccountID.
type CreateRequest struct {
	// AmountKopecks is the gross amount to charge (in kopecks).
	AmountKopecks int64
	// PlatformFeeKopecks is the platform's cut; only meaningful for split payments.
	PlatformFeeKopecks int64
	// Description is the human-readable order description shown to the payer.
	Description string
	// ReturnURL is the redirect target after the payer completes the flow.
	ReturnURL string
	// IdempotencyKey is a stable, opaque string used to de-dup requests at the
	// provider level.  The provider implementation derives its own wire key from
	// this value (e.g. UUID v5) so our internal keys are never exposed.
	IdempotencyKey string
	// SellerAccountID is the marketplace seller's connected account at the
	// provider.  When non-empty the provider creates a split payment with
	// capture=false; when empty a simple immediate-capture charge is created.
	SellerAccountID string
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
	// provider reference and redirect URL.  Split vs. simple routing is
	// determined by req.SellerAccountID.
	CreatePayment(ctx context.Context, req CreateRequest) (*Result, error)

	// Capture confirms a previously authorized payment (two-phase capture).
	// idempotencyKey must be stable across retries for the same capture.
	Capture(ctx context.Context, providerPaymentID, idempotencyKey string) error

	// Refund issues a full refund for an already-captured payment.
	// idempotencyKey must be stable across retries to prevent double-refunds.
	Refund(ctx context.Context, providerPaymentID string, amountKopecks int64, idempotencyKey string) error

	// ParseWebhook parses the raw notification body, applies pull-confirm if
	// required, and returns a provider-agnostic WebhookEvent.  All
	// provider-specific status strings and capture mechanics are hidden here.
	//
	// The rawBody slice must not be retained after ParseWebhook returns.
	ParseWebhook(ctx context.Context, rawBody []byte, headers http.Header) (*domain.WebhookEvent, error)

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
