package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// PaymentUseCase orchestrates payment lifecycle.  It depends on
// provider.PaymentProvider rather than any concrete gateway type, so swapping
// ЮKassa for another provider requires only a wiring change in cmd/main.go.
//
// Event publication is handled via the transactional outbox pattern:
// state transitions write an OutboxEvent row in the same DB transaction as the
// payments UPDATE, eliminating the window where a payment is marked "succeeded"
// but the NATS message is lost.  The outbox worker (internal/outbox/worker.go)
// reads pending rows and publishes them to NATS asynchronously.
type PaymentUseCase struct {
	repo           domain.PaymentRepository
	outbox         domain.OutboxRepository
	ledger         domain.LedgerRepository
	provider       provider.PaymentProvider
	activeProvider string // e.g. "yookassa" — must match Payment.ProviderName on webhook
	returnURL      string
	feeBPS         int64
	holdDuration   time.Duration // accrual hold = ServiceAt + holdDuration
	log            zerolog.Logger
}

// NewPaymentUseCase wires the dependencies.
//
// holdDuration is how long after the service date a successful payment is
// held in escrow before becoming available for payout. 24h is a sensible
// default; configurable via env to widen during ramp-up or chargeback storms.
func NewPaymentUseCase(
	repo domain.PaymentRepository,
	outbox domain.OutboxRepository,
	ledger domain.LedgerRepository,
	prov provider.PaymentProvider,
	activeProvider string,
	returnURL string,
	platformFeeBPS int,
	holdDuration time.Duration,
	lg zerolog.Logger,
) *PaymentUseCase {
	if platformFeeBPS <= 0 {
		platformFeeBPS = 1500
	}
	if holdDuration <= 0 {
		holdDuration = 24 * time.Hour
	}
	return &PaymentUseCase{
		repo:           repo,
		outbox:         outbox,
		ledger:         ledger,
		provider:       prov,
		activeProvider: activeProvider,
		returnURL:      returnURL,
		feeBPS:         int64(platformFeeBPS),
		holdDuration:   holdDuration,
		log:            lg.With().Str("component", "payment-usecase").Logger(),
	}
}

// CreatePaymentInput is the provider-agnostic input for creating a payment.
//
// ServiceAt is the wall-clock moment the booked service begins. The usecase
// passes it to the provider as informational metadata and stores it on the
// payments row so the accrual produced on webhook success can compute its
// own available_at = ServiceAt + payout hold. Optional — when zero the
// accrual falls back to created_at + hold.
type CreatePaymentInput struct {
	BookingID        string
	Amount           int64
	Description      string
	IdempotencyKey   string
	CounterpartyType string
	CounterpartyID   string
	ServiceAt        time.Time
}

// CreatePayment creates a new payment or returns an existing one for the same
// IdempotencyKey.
//
// Amount trust model:
//
//	CreatePayment does NOT re-derive the amount from booking data.  The security
//	boundary is enforced at the transport layer: ServiceTokenServerInterceptor
//	rejects callers without a valid INTERNAL_SERVICE_TOKEN before they reach
//	this function.  Each caller (booking-service, master-service) computes the
//	amount from its own authoritative source.
//
// Idempotency is enforced at the DB level using INSERT … ON CONFLICT DO NOTHING:
//
//   - Two concurrent requests → UNIQUE constraint ensures exactly one INSERT
//     wins; the loser fetches the winner's row.
//   - Crash after INSERT but before provider call → next retry sees isNew=false,
//     ProviderID="" and re-drives the provider call with the same idempotency key.
//   - Crash after provider call but before UpdateProviderInfo → same recovery
//     path; the provider returns the same result for the same idempotency key.
func (uc *PaymentUseCase) CreatePayment(ctx context.Context, in CreatePaymentInput) (*domain.Payment, error) {
	if in.Amount <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	// PlatformFee + CounterpartyNet are frozen on the payments row even though
	// they don't drive the provider call any more — they remain the source of
	// truth for the accrual amount written to partner_ledger on webhook success.
	platformFee := PlatformFeeKopecks(in.Amount, uc.feeBPS)
	net := CounterpartyNetKopecks(in.Amount, platformFee)

	// ── Phase 1: claim the idempotency slot ───────────────────────────────
	p := &domain.Payment{
		BookingID:              in.BookingID,
		Amount:                 in.Amount,
		Status:                 domain.StatusPending,
		IdempotencyKey:         in.IdempotencyKey,
		PlatformFeeKopecks:     platformFee,
		CounterpartyNetKopecks: net,
		CounterpartyType:       strings.TrimSpace(in.CounterpartyType),
		CounterpartyID:         strings.TrimSpace(in.CounterpartyID),
		ServiceAt:              in.ServiceAt,
	}

	isNew, err := uc.repo.CreateIdempotent(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}

	if !isNew {
		// ── Idempotent hit ────────────────────────────────────────────────
		// Fast path: fetch the existing row.  If the provider reference is
		// already filled (the original request completed normally), return it
		// immediately — no lock, no provider call.
		existing, err := uc.repo.GetByIdempotencyKey(ctx, in.IdempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("fetch idempotent payment: %w", err)
		}
		if existing.ProviderID != "" {
			return existing, nil
		}

		// Slow path: ProviderID is empty — the original request crashed between
		// Phase 1 and Phase 3.  Multiple concurrent retries may reach this point
		// simultaneously.  DriveProviderWithLock serialises them with a PostgreSQL
		// advisory lock so the provider is called exactly once.
		return uc.repo.DriveProviderWithLock(ctx, in.IdempotencyKey, func(p *domain.Payment) (string, string, error) {
			result, err := uc.provider.CreatePayment(ctx, provider.CreateRequest{
				AmountKopecks:  in.Amount,
				Description:    in.Description,
				ReturnURL:      uc.returnURL,
				IdempotencyKey: in.IdempotencyKey,
				CounterpartyID: strings.TrimSpace(in.CounterpartyID),
				BookingID:      in.BookingID,
			})
			if err != nil {
				return "", "", fmt.Errorf("provider create payment (recovery): %w", err)
			}
			return result.ProviderPaymentID, result.ConfirmationURL, nil
		})
	}

	// ── Phase 2: call the payment provider (new row, no contention) ───────
	// isNew=true means we are the sole owner of this idempotency slot —
	// no lock needed, no concurrent retries are possible yet.
	result, err := uc.provider.CreatePayment(ctx, provider.CreateRequest{
		AmountKopecks:  in.Amount,
		Description:    in.Description,
		ReturnURL:      uc.returnURL,
		IdempotencyKey: in.IdempotencyKey,
		CounterpartyID: strings.TrimSpace(in.CounterpartyID),
		BookingID:      in.BookingID,
	})
	if err != nil {
		return nil, fmt.Errorf("provider create payment: %w", err)
	}

	// ── Phase 3: persist the provider reference ───────────────────────────
	if err := uc.repo.UpdateProviderInfo(ctx, p.ID, result.ProviderPaymentID, result.ConfirmationURL); err != nil {
		return nil, fmt.Errorf("update provider info: %w", err)
	}

	p.ProviderID = result.ProviderPaymentID
	p.PaymentURL = result.ConfirmationURL
	return p, nil
}

func (uc *PaymentUseCase) GetPayment(ctx context.Context, id string) (*domain.Payment, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *PaymentUseCase) GetByBooking(ctx context.Context, bookingID string) (*domain.Payment, error) {
	return uc.repo.GetByBookingID(ctx, bookingID)
}

// ── Webhook handling ─────────────────────────────────────────────────────────

// HandleWebhook processes a raw inbound provider notification.
//
// The usecase is provider-agnostic: all wire-format parsing, pull-confirm logic,
// and two-phase capture mechanics are delegated to provider.ParseWebhook.
// The usecase only dispatches on the resulting domain.WebhookEvent:
//
//   - RequiresCapture=true  → call provider.Capture, then return.
//   - Status terminal       → UpdateStatusWithOutbox (atomic DB + outbox write).
//   - Status pending (no-op)→ return nil (also when provider returns an
//     unrecognised status that did not map to a domain value — see RawProviderStatus).
//
// Idempotency is enforced at the DB layer (UpdateStatus WHERE status NOT IN
// terminal states), so duplicate deliveries are silently absorbed.
//
// The returned *domain.WebhookEvent is always non-nil on success and is
// intended for structured logging at the delivery layer.  It carries
// RawEvent and RawProviderStatus so the caller can log permitted fields
// without re-parsing the raw body.
func (uc *PaymentUseCase) HandleWebhook(ctx context.Context, rawBody []byte) (*domain.WebhookEvent, error) {
	event, err := uc.provider.ParseWebhook(ctx, rawBody, nil)
	if err != nil {
		return nil, fmt.Errorf("parse webhook: %w", err)
	}

	if event.RequiresCapture {
		// No provider-mismatch guard here: Capture is a no-op at the provider API
		// if the ID doesn't exist (wrong-provider replay), and it performs no DB
		// write on its own, so there is no state-corruption risk.  A forged capture
		// request fails at the provider level with a 4xx that isPermanent returns
		// true for, so the error propagates back without retry.
		if err := uc.provider.Capture(ctx, event.ProviderPaymentID, event.CaptureKey); err != nil {
			return event, fmt.Errorf("capture payment: %w", err)
		}
		return event, nil
	}

	if !event.Status.IsTerminal() {
		// Non-terminal: either a known non-terminal provider status (pending,
		// waiting_for_capture) or an unrecognised status coerced to pending by
		// the provider.  Warn on the latter so operators notice new ЮKassa
		// statuses before they silently become no-ops in production.
		if event.RawProviderStatus != "" {
			if _, known := knownProviderStatuses[event.RawProviderStatus]; !known {
				uc.log.Warn().
					Str("object_id", event.ProviderPaymentID).
					Str("raw_provider_status", event.RawProviderStatus).
					Msg("webhook: unrecognised provider status, treating as pending no-op")
			}
		}
		return event, nil
	}

	p, err := uc.repo.GetByProviderID(ctx, event.ProviderPaymentID)
	if err != nil {
		return event, fmt.Errorf("payment not found for provider_id %s: %w", event.ProviderPaymentID, err)
	}

	// Provider-mismatch guard: reject a webhook whose payment row was created by
	// a different provider than the one currently active.
	//
	// Scenario this prevents: service is migrated from ЮKassa to T-Bank; a stale
	// ЮKassa notification arrives after the switchover (or an attacker replays one).
	// The active provider parsed it successfully (both providers may accept
	// structurally valid JSON), but the stored ProviderName disagrees — applying
	// the status update would corrupt a T-Bank payment record with ЮKassa data.
	//
	// In mock mode activeProvider is empty — skip the check to keep tests simple.
	if uc.activeProvider != "" && p.ProviderName != uc.activeProvider {
		uc.log.Warn().
			Str("object_id", event.ProviderPaymentID).
			Str("payment_provider", p.ProviderName).
			Str("active_provider", uc.activeProvider).
			Msg("webhook: provider mismatch — ignoring notification")
		return event, status.Errorf(codes.FailedPrecondition,
			"webhook provider mismatch: payment created by %q, active provider is %q",
			p.ProviderName, uc.activeProvider,
		)
	}

	subject := statusToSubject(event.Status)
	outboxEvent, err := buildOutboxEvent(p, subject)
	if err != nil {
		return event, fmt.Errorf("build outbox event: %w", err)
	}

	updated, err := uc.repo.UpdateStatusWithOutbox(ctx, p.ID, event.Status, event.ProviderPaymentID, outboxEvent)
	if err != nil {
		return event, err
	}
	if !updated {
		// Already in a terminal state — duplicate delivery, absorb silently.
		return event, nil
	}

	// On terminal-success: credit the partner's ledger with their net share.
	// Idempotent — the partial unique index on (payment_id) WHERE entry_type=accrual
	// means a retried successful webhook collapses to one accrual.
	if event.Status == domain.StatusSucceeded {
		if err := uc.writeAccrual(ctx, p); err != nil {
			// Failing here would leave the payment terminal-succeeded but the
			// partner uncredited. Return the error so the webhook caller Naks
			// and ЮKassa retries; the UpdateStatus guard absorbs the duplicate
			// status flip, and the ledger guard absorbs the duplicate accrual.
			return event, fmt.Errorf("write accrual: %w", err)
		}
	}

	return event, nil
}

// writeAccrual records the partner's share of a successful payment in the
// append-only ledger.  No-op when the payment has no counterparty (e.g. a
// future direct platform fee) — the accrual would be ambiguous with no
// partner to credit.
//
// available_at = ServiceAt + holdDuration when ServiceAt is set; otherwise
// falls back to now + holdDuration so the entry still respects the hold
// window for service-less charges.
func (uc *PaymentUseCase) writeAccrual(ctx context.Context, p *domain.Payment) error {
	if p.CounterpartyID == "" || p.CounterpartyType == "" {
		return nil
	}
	if p.CounterpartyNetKopecks <= 0 {
		return nil
	}

	baseAt := p.ServiceAt
	if baseAt.IsZero() {
		baseAt = time.Now()
	}

	entry := &domain.LedgerEntry{
		PartnerType:   domain.PartnerType(p.CounterpartyType),
		PartnerID:     p.CounterpartyID,
		EntryType:     domain.LedgerAccrual,
		AmountKopecks: p.CounterpartyNetKopecks,
		PaymentID:     p.ID,
		AvailableAt:   baseAt.Add(uc.holdDuration),
	}
	if err := uc.ledger.AppendAccrual(ctx, entry); err != nil {
		// AlreadyExists is the idempotent retry path — absorb silently.
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		return err
	}
	return nil
}

// knownProviderStatuses is the set of raw provider status strings that the
// client is expected to return from ParseWebhook.  Used by HandleWebhook to
// distinguish "genuine non-terminal" from "unrecognised status silently coerced
// to pending" so the latter can be warned on rather than silently absorbed.
//
// This must stay in sync with yookassa.statusToDomain and the ЮKassa docs.
// It is intentionally kept as a usecase-layer constant (not imported from the
// provider package) to avoid coupling the usecase to the ЮKassa wire format.
var knownProviderStatuses = map[string]struct{}{
	"pending":             {}, // payment created, awaiting user action
	"waiting_for_capture": {}, // authorised, usecase calls Capture
	"succeeded":           {}, // terminal: funds moved
	"canceled":            {}, // terminal: payment cancelled (ЮKassa spelling)
}

// RefundByBooking issues a full refund for the payment associated with
// bookingID.  Called by the NATS subscriber on booking.cancelled.
//
// State machine:
//   - payment not found  → no-op (cancelled before payment was created)
//   - status != Succeeded → no-op (pending/cancelled/refunded — nothing to refund)
//   - status == Succeeded → Refund at provider, then UpdateStatusWithOutbox → Refunded
//
// Provider errors are returned — the subscriber must Nak() and retry.
// Re-delivery is safe: Refund uses a stable idempotency key so the provider
// returns the same result without double-charging.
func (uc *PaymentUseCase) RefundByBooking(ctx context.Context, bookingID string) error {
	p, err := uc.repo.GetByBookingID(ctx, bookingID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("get payment by booking: %w", err)
	}

	if p.Status != domain.StatusSucceeded {
		return nil
	}

	idempotencyKey := p.ID + "-refund"
	if err := uc.provider.Refund(ctx, p.ProviderID, p.Amount, idempotencyKey); err != nil {
		return fmt.Errorf("refund booking %s: %w", bookingID, err)
	}

	// Update p.Status so buildOutboxEvent sees the new status.
	p.Status = domain.StatusRefunded
	outboxEvent, err := buildOutboxEvent(p, domain.SubjectPaymentRefunded)
	if err != nil {
		return fmt.Errorf("build outbox event for refund: %w", err)
	}

	if _, err := uc.repo.UpdateStatusWithOutbox(ctx, p.ID, domain.StatusRefunded, p.ProviderID, outboxEvent); err != nil {
		// Refund succeeded at provider; if this write fails the subscriber will
		// Nak() and retry.  The next Refund call with the same key is a no-op at
		// the provider — no double-refund risk.
		return fmt.Errorf("update payment status to refunded: %w", err)
	}

	// Reverse the partner's accrual.  Idempotent on (payment_id) via the
	// partial unique index — retried refunds collapse to one reversal.
	if err := uc.writeReversal(ctx, p); err != nil {
		return fmt.Errorf("write reversal: %w", err)
	}

	return nil
}

// writeReversal records a refund reversal in the ledger, undoing the original
// accrual.  No-op when no accrual exists (refund of a payment that never
// reached succeeded — shouldn't happen given the p.Status check above, but
// kept defensive).
func (uc *PaymentUseCase) writeReversal(ctx context.Context, p *domain.Payment) error {
	accrual, err := uc.ledger.FindAccrualByPayment(ctx, p.ID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("find accrual: %w", err)
	}
	entry := &domain.LedgerEntry{
		PartnerType:     accrual.PartnerType,
		PartnerID:       accrual.PartnerID,
		EntryType:       domain.LedgerReversal,
		AmountKopecks:   -accrual.AmountKopecks,
		PaymentID:       p.ID,
		ReversesEntryID: accrual.ID,
		Reason:          "refund",
	}
	if err := uc.ledger.AppendReversal(ctx, entry); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		return err
	}
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// paymentEventPayload is the JSON shape written to the outbox and subsequently
// published to NATS.
//
// Only the fields downstream consumers actually read are included. Both current
// subscribers — booking-service and master-service — decode just payment_id,
// booking_id and status (they confirm/cancel a booking; they never look at
// money). Financial fields (amount, platform fee, counterparty net) are
// deliberately omitted: broadcasting them to the event bus would leak payment
// amounts to services that have no use for them, with no benefit. If a future
// consumer genuinely needs an amount, add the specific field back here together
// with the consumer that reads it — don't pre-emptively widen the payload.
type paymentEventPayload struct {
	PaymentID string `json:"payment_id"`
	BookingID string `json:"booking_id"`
	Status    string `json:"status"`
}

func buildOutboxEvent(p *domain.Payment, subject domain.OutboxSubject) (*domain.OutboxEvent, error) {
	payload, err := json.Marshal(paymentEventPayload{
		PaymentID: p.ID,
		BookingID: p.BookingID,
		Status:    p.Status.String(),
	})
	if err != nil {
		return nil, err
	}
	return &domain.OutboxEvent{
		Subject:   subject,
		Payload:   payload,
		PaymentID: p.ID,
	}, nil
}

func statusToSubject(s domain.PaymentStatus) domain.OutboxSubject {
	switch s {
	case domain.StatusSucceeded:
		return domain.SubjectPaymentCompleted
	case domain.StatusCancelled:
		return domain.SubjectPaymentFailed
	case domain.StatusRefunded:
		return domain.SubjectPaymentRefunded
	default:
		return domain.SubjectPaymentFailed
	}
}
