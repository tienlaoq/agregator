package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

// PayoutRepo persists outbound payouts.
//
// Concurrency model:
//
//	The payouts_partner_active_uniq partial index (migration 013) guarantees
//	at most one (pending|processing) payout per partner. CreatePendingWithLedger
//	relies on it: a concurrent second attempt fails at commit with 23505 and
//	is translated to AlreadyExists. Callers should retry on the next scheduler
//	tick rather than spinning here.
//
//	payouts_idempotency_key_uniq is the second guard: deterministic keys
//	(partner + run timestamp) make a retried scheduler tick collapse to one
//	provider call.
type PayoutRepo struct {
	pool *pgxpool.Pool
}

func NewPayoutRepo(pool *pgxpool.Pool) *PayoutRepo {
	return &PayoutRepo{pool: pool}
}

const payoutSelectCols = `
	id, partner_type, partner_id, amount_kopecks, currency,
	method_id, method_kind_snapshot, method_display,
	status, provider_name, COALESCE(provider_payout_id, ''), idempotency_key,
	COALESCE(failure_reason, ''), attempt_count,
	created_at, updated_at, completed_at`

// CreatePendingWithLedger inserts the payout row (status=pending) and the
// matching negative ledger entry in one transaction.
//
// Both writes share one transaction, so the ledger debit becomes visible to
// Balance() the moment the payout exists — Balance/CreatePayout races cannot
// double-spend. The partial unique index on (partner_type, partner_id)
// WHERE status IN ('pending', 'processing') is the second line of defence:
// even if Balance was stale, only one open payout can survive commit.
func (r *PayoutRepo) CreatePendingWithLedger(ctx context.Context, p *domain.Payout) error {
	if !p.PartnerType.Valid() {
		return pkgerr.InvalidArgument("invalid partner_type")
	}
	if p.AmountKopecks <= 0 {
		return pkgerr.InvalidArgument("payout amount must be positive")
	}
	if p.MethodID == "" {
		return pkgerr.InvalidArgument("payout requires method_id")
	}
	if p.IdempotencyKey == "" {
		return pkgerr.InvalidArgument("payout requires idempotency_key")
	}
	if p.ProviderName == "" {
		return pkgerr.InvalidArgument("payout requires provider_name")
	}
	if p.Currency == "" {
		p.Currency = "RUB"
	}
	if p.Status == "" {
		p.Status = domain.PayoutPending
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("payout create begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize all ledger writes for this partner against concurrent refunds.
	//
	// Race the lock prevents:
	//   T0: usecase reads Balance() = 50_000.
	//   T1: refund-webhook AppendReversal -10_000 commits.  Real balance = 40_000.
	//   T2: scheduler INSERT payout=50_000 + ledger -50_000 → -10_000 (negative).
	//
	// The "partner:<type>:<id>" advisory lock is also taken by AppendReversal —
	// the two operations therefore can never overlap.  Inside the lock we
	// re-read the live balance and refuse the payout if the partner has been
	// drained since enumeration.
	if err = acquirePartnerLock(ctx, tx, p.PartnerType, p.PartnerID); err != nil {
		return fmt.Errorf("payout partner lock: %w", err)
	}

	available, err := readAvailableBalance(ctx, tx, p.PartnerType, p.PartnerID)
	if err != nil {
		return fmt.Errorf("payout balance recheck: %w", err)
	}
	if available < p.AmountKopecks {
		// Stale-balance race — refund or another payout drained the partner.
		// FailedPrecondition is the right gRPC code: the request is well-formed,
		// but the precondition (available >= amount) no longer holds.
		return pkgerr.FailedPrecondition(fmt.Sprintf(
			"payout precondition: available=%d < requested=%d", available, p.AmountKopecks))
	}

	// id is honoured when pre-set by the caller (idempotency-key derivation
	// upstream depends on it).  Empty id falls back to gen_random_uuid().
	const insertPayout = `
		INSERT INTO payouts (
			id,
			partner_type, partner_id, amount_kopecks, currency,
			method_id, method_kind_snapshot, method_display,
			status, provider_name, idempotency_key
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()),
			$2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
		RETURNING id, created_at, updated_at`

	row := tx.QueryRow(ctx, insertPayout,
		p.ID,
		string(p.PartnerType), p.PartnerID, p.AmountKopecks, p.Currency,
		p.MethodID, string(p.MethodKindSnapshot), p.MethodDisplay,
		string(p.Status), p.ProviderName, p.IdempotencyKey,
	)
	if err = row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			// Caught by either idempotency_key_uniq or partner_active_uniq.
			return pkgerr.AlreadyExists("payout for partner already exists or is in flight")
		}
		return fmt.Errorf("payout insert: %w", err)
	}

	// Mirror the debit in the ledger so Balance() updates atomically with
	// the payout row becoming visible. amount stored negative as per the
	// chk_amount_sign constraint.
	debit := &domain.LedgerEntry{
		PartnerType:   p.PartnerType,
		PartnerID:     p.PartnerID,
		EntryType:     domain.LedgerPayout,
		AmountKopecks: -p.AmountKopecks,
		PayoutID:      p.ID,
	}
	if err = appendLedgerEntry(ctx, tx, debit); err != nil {
		return fmt.Errorf("payout ledger debit: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("payout create commit: %w", err)
	}
	return nil
}

// MarkProcessing flips pending → processing and records the provider's
// payout id. Idempotent on retries: a row already in processing or terminal
// is left untouched and the call returns nil.
func (r *PayoutRepo) MarkProcessing(ctx context.Context, id, providerPayoutID string) error {
	const q = `
		UPDATE payouts
		   SET status = 'processing', provider_payout_id = $2,
		       attempt_count = attempt_count + 1, updated_at = now()
		 WHERE id = $1 AND status = 'pending'`
	if _, err := r.pool.Exec(ctx, q, id, providerPayoutID); err != nil {
		return fmt.Errorf("payout mark processing: %w", err)
	}
	return nil
}

// MarkSucceeded transitions a payout to the terminal succeeded state.
// Returns updated=false when the row is already terminal (duplicate webhook),
// matching the contract used by payments.UpdateStatusWithOutbox.
func (r *PayoutRepo) MarkSucceeded(ctx context.Context, id string, completedAt time.Time) (bool, error) {
	const q = `
		UPDATE payouts
		   SET status = 'succeeded', completed_at = $2, updated_at = now()
		 WHERE id = $1 AND status NOT IN ('succeeded', 'failed')`
	ct, err := r.pool.Exec(ctx, q, id, completedAt)
	if err != nil {
		return false, fmt.Errorf("payout mark succeeded: %w", err)
	}
	return ct.RowsAffected() > 0, nil
}

// MarkFailedWithReversal atomically:
//   - flips the payout to failed (and records the reason),
//   - writes a positive 'reversal' ledger entry referencing the payout
//     so the partner's balance is restored (the original 'payout' entry
//     stays as audit trail).
//
// Idempotent: if the payout is already terminal, returns updated=false
// without writing any reversal.
func (r *PayoutRepo) MarkFailedWithReversal(ctx context.Context, id, reason string, completedAt time.Time) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("payout fail begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the row and read the data we need for the compensating ledger entry.
	const lockQ = `
		SELECT partner_type, partner_id, amount_kopecks, status
		FROM payouts WHERE id = $1 FOR UPDATE`
	var (
		partnerType, statusStr string
		partnerID              string
		amount                 int64
	)
	if err = tx.QueryRow(ctx, lockQ, id).Scan(&partnerType, &partnerID, &amount, &statusStr); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, pkgerr.NotFound("payout not found")
		}
		return false, fmt.Errorf("payout fail lock: %w", err)
	}
	if domain.PayoutStatus(statusStr).IsTerminal() {
		return false, nil
	}

	// Same partner-advisory-lock as AppendReversal / CreatePendingWithLedger.
	// A failed payout reverses the original ledger debit (positive 'adjustment'
	// entry); without the lock, a concurrent CreatePendingWithLedger could read
	// stale balance and over-pay.
	if err = acquirePartnerLock(ctx, tx, domain.PartnerType(partnerType), partnerID); err != nil {
		return false, fmt.Errorf("payout fail partner lock: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE payouts
		   SET status = 'failed', failure_reason = $2, completed_at = $3, updated_at = now()
		 WHERE id = $1`, id, reason, completedAt); err != nil {
		return false, fmt.Errorf("payout fail update: %w", err)
	}

	// Compensating ledger entry. Positive amount (refunds the partner) tied
	// to the payout via payout_id so audit trail is preserved. We store this
	// as an 'adjustment' rather than 'reversal' because 'reversal' is reserved
	// for reversing accruals (refunds), and the partial unique index on
	// payout_id allows only one 'payout' entry — a positive entry under the
	// same payout_id with type 'adjustment' is unconstrained.
	reasonText := reason
	if reasonText == "" {
		reasonText = "payout failed"
	}
	credit := &domain.LedgerEntry{
		PartnerType:   domain.PartnerType(partnerType),
		PartnerID:     partnerID,
		EntryType:     domain.LedgerAdjustment,
		AmountKopecks: amount,
		PayoutID:      id,
		Reason:        "payout-failed-reversal: " + reasonText,
	}
	if err = appendLedgerEntry(ctx, tx, credit); err != nil {
		return false, fmt.Errorf("payout fail compensating ledger: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("payout fail commit: %w", err)
	}
	return true, nil
}

func (r *PayoutRepo) GetByID(ctx context.Context, id string) (*domain.Payout, error) {
	q := `SELECT ` + payoutSelectCols + ` FROM payouts WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

func (r *PayoutRepo) GetByProviderPayoutID(ctx context.Context, providerPayoutID string) (*domain.Payout, error) {
	q := `SELECT ` + payoutSelectCols + ` FROM payouts WHERE provider_payout_id = $1`
	return r.scanOne(ctx, q, providerPayoutID)
}

// ListPendingOlderThan returns pending payouts that have been waiting longer
// than olderThan since creation.  Used by the reconciliation worker.
//
// Filter on status='pending' (NOT 'processing') because processing means the
// provider has accepted the request and given us a provider_payout_id — that
// flow is webhook-driven and self-recovers.  pending means we never got a
// response from the provider; those are the rows that need re-driving.
func (r *PayoutRepo) ListPendingOlderThan(ctx context.Context, olderThan time.Duration, limit int) ([]domain.Payout, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	cutoff := time.Now().Add(-olderThan)

	q := `SELECT ` + payoutSelectCols + `
		FROM payouts
		WHERE status = 'pending' AND created_at <= $1
		ORDER BY created_at ASC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("payout list pending query: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Payout, 0, limit)
	for rows.Next() {
		p, err := scanPayoutFromRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("payout list pending rows: %w", err)
	}
	return out, nil
}

func (r *PayoutRepo) List(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.Payout, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	q := `SELECT ` + payoutSelectCols + `
		FROM payouts
		WHERE partner_type = $1 AND partner_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.pool.Query(ctx, q, string(partnerType), partnerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("payout list query: %w", err)
	}
	defer rows.Close()

	out := make([]domain.Payout, 0, limit)
	for rows.Next() {
		p, err := scanPayoutFromRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("payout list rows: %w", err)
	}
	return out, nil
}

// LastPayoutAt returns the created_at of the partner's most recent non-failed
// payout. Failed payouts are excluded because their ledger debit was reversed —
// they should not push the partner's next payout a week out.
func (r *PayoutRepo) LastPayoutAt(ctx context.Context, partnerType domain.PartnerType, partnerID string) (time.Time, bool, error) {
	const q = `SELECT created_at
		FROM payouts
		WHERE partner_type = $1 AND partner_id = $2 AND status <> 'failed'
		ORDER BY created_at DESC
		LIMIT 1`

	var at time.Time
	err := r.pool.QueryRow(ctx, q, string(partnerType), partnerID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("payout last-at query: %w", err)
	}
	return at, true, nil
}

func (r *PayoutRepo) scanOne(ctx context.Context, q string, args ...any) (*domain.Payout, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if rows.Err() != nil {
			return nil, rows.Err()
		}
		return nil, pkgerr.NotFound("payout not found")
	}
	return scanPayoutFromRow(rows)
}

// scanPayoutFromRow extracts a Payout from either a *pgx.Rows or pgx.Row
// surface — both satisfy the implicit "Scan(...) error" method set.
func scanPayoutFromRow(rows pgx.Rows) (*domain.Payout, error) {
	p := &domain.Payout{}
	var partnerType, methodKind, statusStr string
	var completedAt *time.Time
	if err := rows.Scan(
		&p.ID, &partnerType, &p.PartnerID, &p.AmountKopecks, &p.Currency,
		&p.MethodID, &methodKind, &p.MethodDisplay,
		&statusStr, &p.ProviderName, &p.ProviderPayoutID, &p.IdempotencyKey,
		&p.FailureReason, &p.AttemptCount,
		&p.CreatedAt, &p.UpdatedAt, &completedAt,
	); err != nil {
		return nil, fmt.Errorf("payout scan: %w", err)
	}
	p.PartnerType = domain.PartnerType(partnerType)
	p.MethodKindSnapshot = domain.PayoutMethodKind(methodKind)
	p.Status = domain.PayoutStatus(statusStr)
	if completedAt != nil {
		p.CompletedAt = *completedAt
	}
	return p, nil
}
