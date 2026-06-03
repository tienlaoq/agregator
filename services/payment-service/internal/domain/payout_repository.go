package domain

import (
	"context"
	"time"
)

// PayoutMethodRepository persists partner bank rails.  Upsert semantics:
// Save deactivates any prior active method for the same (partner_type, partner_id)
// and inserts a new active row in a single transaction.
type PayoutMethodRepository interface {
	// Save activates m as the partner's current payout method.  Any previously
	// active method for the same partner is marked DeactivatedAt=now() in the
	// same transaction.  On return m.ID, m.CreatedAt, m.UpdatedAt are populated.
	Save(ctx context.Context, m *PayoutMethod) error

	// GetActive returns the partner's currently active method.
	// Returns a NotFound status error when no active method exists.
	GetActive(ctx context.Context, partnerType PartnerType, partnerID string) (*PayoutMethod, error)

	// GetByID returns a specific method by id (active or not).  Used to read
	// the method snapshot referenced by a payout row.
	GetByID(ctx context.Context, id string) (*PayoutMethod, error)
}

// LedgerRepository is the append-only journal of partner money movements.
// All writes are inserts; entries are never updated or deleted (immutability
// is enforced by convention at the usecase layer — there is no UPDATE method).
type LedgerRepository interface {
	// AppendAccrual writes a positive accrual referencing paymentID.  Returns
	// AlreadyExists status error if an accrual for this payment is already
	// present (idempotent on retried webhooks).
	AppendAccrual(ctx context.Context, e *LedgerEntry) error

	// AppendReversal writes a negative reversal referencing the original
	// accrual via ReversesEntryID.  Idempotent on (payment_id, type='reversal').
	AppendReversal(ctx context.Context, e *LedgerEntry) error

	// FindAccrualByPayment returns the accrual entry recorded for paymentID,
	// or NotFound if none exists.  Used when the usecase needs the entry id
	// and amount before issuing a reversal.
	FindAccrualByPayment(ctx context.Context, paymentID string) (*LedgerEntry, error)

	// Balance computes (total, available, held) for the partner via a single
	// SUM query.  available counts only entries that are either non-accrual
	// or have available_at <= now.
	Balance(ctx context.Context, partnerType PartnerType, partnerID string) (*PartnerBalance, error)

	// List returns entries for the partner ordered by id DESC, paginated.
	List(ctx context.Context, partnerType PartnerType, partnerID string, limit, offset int) ([]LedgerEntry, error)

	// PartnersWithAvailableBalance returns up to limit partner identifiers
	// whose available balance is ≥ minKopecks.  Used by the payout scheduler.
	// The query reads the ripe-accruals partial index and aggregates server-side.
	PartnersWithAvailableBalance(ctx context.Context, minKopecks int64, limit int) ([]PartnerRef, error)
}

// PartnerRef is a (type, id) pair used by the scheduler to enumerate work.
type PartnerRef struct {
	PartnerType PartnerType
	PartnerID   string
}

// PayoutRepository persists outbound payouts.
type PayoutRepository interface {
	// CreatePendingWithLedger atomically inserts the payout row (status=pending)
	// and a matching ledger entry (type='payout', amount=-amount, payout_id=...).
	// Both writes share one transaction; the ledger debit is therefore visible
	// to Balance() the instant the payout is created — preventing double-spend
	// when the scheduler races against itself.
	//
	// idempotencyKey is unique; a duplicate returns AlreadyExists.
	CreatePendingWithLedger(ctx context.Context, p *Payout) error

	// MarkProcessing transitions pending → processing and records the provider
	// payout id.  No-op on retries (idempotent on status).
	MarkProcessing(ctx context.Context, id, providerPayoutID string) error

	// MarkSucceeded transitions to succeeded.  Returns updated=false when the
	// payout is already terminal (duplicate webhook).
	MarkSucceeded(ctx context.Context, id string, completedAt time.Time) (updated bool, err error)

	// MarkFailedWithReversal atomically transitions to failed AND inserts a
	// reversal ledger entry (positive amount, references the payout via payout_id)
	// so the partner's balance is restored.
	MarkFailedWithReversal(ctx context.Context, id, reason string, completedAt time.Time) (updated bool, err error)

	GetByID(ctx context.Context, id string) (*Payout, error)
	GetByProviderPayoutID(ctx context.Context, providerPayoutID string) (*Payout, error)

	// List returns payouts for the partner ordered by created_at DESC.
	List(ctx context.Context, partnerType PartnerType, partnerID string, limit, offset int) ([]Payout, error)

	// ListPendingOlderThan returns pending payouts that have been waiting longer
	// than olderThan since creation.  Used by the reconciliation worker to detect
	// payouts whose CreatePayout call returned a transient error (the provider
	// may or may not have accepted the request) and re-drive them via the same
	// idempotency key.
	//
	// Returns ordered by created_at ASC so the oldest stuck payouts are
	// reconciled first.  Capped at limit.
	ListPendingOlderThan(ctx context.Context, olderThan time.Duration, limit int) ([]Payout, error)
}
