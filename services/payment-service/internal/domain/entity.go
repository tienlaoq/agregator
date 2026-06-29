package domain

import "time"

// PaymentStatus is the set of lifecycle states a payment can be in.
// Using a named string type (rather than int iota) keeps DB values and JSON
// representations human-readable without a separate stringer, while still
// giving the compiler visibility into invalid assignments.
type PaymentStatus string

const (
	// StatusPending is the initial state: the payment row exists but the
	// provider has not yet confirmed any outcome.
	StatusPending PaymentStatus = "pending"

	// StatusSucceeded means the provider confirmed a successful charge.
	// Terminal — no further transitions are expected.
	StatusSucceeded PaymentStatus = "succeeded"

	// StatusCancelled means the provider or the platform cancelled the payment
	// before funds were captured.  Terminal.
	StatusCancelled PaymentStatus = "cancelled"

	// StatusRefunded means a full refund was issued via the provider after the
	// payment had already succeeded.  Terminal.
	//
	// NOTE: this value was missing from the original CHECK constraint in
	// 001_init.up.sql — migration 004_status_refunded.up.sql adds it.
	StatusRefunded PaymentStatus = "refunded"
)

// IsTerminal reports whether the status is a terminal state from which no
// further transitions should occur.  Used by the repository UPDATE guard and
// webhook idempotency logic.
func (s PaymentStatus) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusCancelled || s == StatusRefunded
}

// String returns the raw DB/wire value.  Satisfies fmt.Stringer.
func (s PaymentStatus) String() string { return string(s) }

type Payment struct {
	ID                     string
	BookingID              string
	Amount                 int64
	Status                 PaymentStatus
	ProviderID             string
	PaymentURL             string
	IdempotencyKey         string
	PlatformFeeKopecks     int64
	CounterpartyNetKopecks int64
	CounterpartyType       string // "venue" | "master"
	CounterpartyID         string
	ProviderName           string // e.g. "mock", "tbank"
	// ServiceAt is the wall-clock moment the booked service begins.  Used to
	// compute the accrual's available_at (= ServiceAt + payout hold).  Zero
	// value means "no service date" — accrual falls back to created_at + hold.
	ServiceAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WebhookEvent is the provider-agnostic result of parsing and validating an
// inbound webhook notification.  It is produced by the payment provider and
// consumed by PaymentUseCase.HandleWebhook.
//
// The usecase never interprets provider-specific wire formats, status strings,
// or two-phase capture mechanics — all of that is encapsulated here.
type WebhookEvent struct {
	// ProviderPaymentID is the provider's opaque payment identifier, used to
	// look up the local Payment record.
	ProviderPaymentID string

	// Status is the authoritative domain status after the provider has been
	// pull-confirmed.  StatusPending means the payment has not yet reached a
	// terminal state and no action is required (the usecase should no-op).
	Status PaymentStatus

	// RequiresCapture is true when the provider is waiting for an explicit
	// capture call before funds are moved.  When true the usecase must call
	// provider.Capture(ctx, ProviderPaymentID, CaptureKey) before returning.
	//
	// Example: a two-phase gateway uses capture=false and sends a
	// "waiting_for_capture" notification which the provider translates into this
	// flag (e.g. a Tinkoff provider would set it after its Init step).
	RequiresCapture bool

	// CaptureKey is the idempotency key the usecase must pass to Capture.
	// It is populated by the provider when RequiresCapture is true; the
	// usecase treats it as an opaque string.
	CaptureKey string

	// RawEvent is the provider-specific event type string extracted from the
	// webhook payload (e.g. "payment.succeeded").  Populated by the provider
	// for structured logging at the delivery layer; never used for routing.
	RawEvent string

	// RawProviderStatus is the status string extracted from the webhook payload
	// before pull-confirm (e.g. "succeeded", "canceled", "waiting_for_capture").
	// This is the payload hint, not the pull-confirmed authoritative status.
	// Populated by the provider for logging; never used for business logic.
	RawProviderStatus string
}
