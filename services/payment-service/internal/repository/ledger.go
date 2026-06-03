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

// LedgerRepo is the append-only partner ledger.
//
// All writes are pure INSERTs; updates and deletes are intentionally absent
// from the API surface so the file-level invariant ("rows are immutable
// financial records") is enforced by absence.
//
// Idempotency for the accrual/reversal/payout entries is delegated to the
// partial unique indexes installed by migration 012; a duplicate write
// surfaces as 23505 (unique_violation) which we translate to AlreadyExists
// so the caller can absorb the retry silently.
type LedgerRepo struct {
	pool *pgxpool.Pool
}

func NewLedgerRepo(pool *pgxpool.Pool) *LedgerRepo {
	return &LedgerRepo{pool: pool}
}

const pgUniqueViolation = "23505"

// partnerLockExecutor is the narrow surface needed to acquire the partner
// advisory lock — *pgxpool.Pool and pgx.Tx both satisfy it.  Using the
// interface keeps acquirePartnerLock callable from any context (TX or pool).
type partnerLockExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// acquirePartnerLock takes a transaction-scoped PostgreSQL advisory lock
// keyed on (partner_type, partner_id).  The lock auto-releases when the
// surrounding transaction commits or rolls back.
//
// Purpose: serialize every ledger-writing operation for a single partner so
// that a refund-reversal cannot land between a scheduler's balance read and
// its payout insert.  Both PayoutRepo.CreatePendingWithLedger and
// LedgerRepo.AppendReversal acquire this lock; without it, payouts could
// drain a balance that has just been reduced by a concurrent refund.
//
// hashtext returns int4, which the int8 pg_advisory_xact_lock overload
// accepts via implicit cast.  Collision space is 32-bit — distinct from the
// payment idempotency lock space because the input strings are prefixed
// differently ("partner:..." vs the raw idempotency key).
func acquirePartnerLock(ctx context.Context, exec partnerLockExecutor, partnerType domain.PartnerType, partnerID string) error {
	key := "partner:" + string(partnerType) + ":" + partnerID
	_, err := exec.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, key)
	return err
}

// readAvailableBalance computes the partner's available balance inside the
// caller's transaction.  Mirrors LedgerRepo.Balance's available expression
// (entry_type<>'accrual' OR available_at<=now()) so the value returned here
// is the same one the scheduler uses for its planning step — read-after-write
// consistency is guaranteed by the surrounding transaction.
//
// balanceExecutor mirrors ledgerExecutor: both pgxpool.Pool and pgx.Tx fit.
func readAvailableBalance(ctx context.Context, exec ledgerExecutor, partnerType domain.PartnerType, partnerID string) (int64, error) {
	const q = `
		SELECT COALESCE(SUM(CASE
			WHEN entry_type <> 'accrual' THEN amount_kopecks
			WHEN available_at IS NOT NULL AND available_at <= now() THEN amount_kopecks
			ELSE 0
		END), 0)
		FROM partner_ledger
		WHERE partner_type = $1 AND partner_id = $2`
	var available int64
	if err := exec.QueryRow(ctx, q, string(partnerType), partnerID).Scan(&available); err != nil {
		return 0, err
	}
	return available, nil
}

func (r *LedgerRepo) AppendAccrual(ctx context.Context, e *domain.LedgerEntry) error {
	if e.EntryType != domain.LedgerAccrual {
		return pkgerr.InvalidArgument("AppendAccrual requires entry_type=accrual")
	}
	if e.AmountKopecks <= 0 {
		return pkgerr.InvalidArgument("accrual amount must be positive")
	}
	if e.PaymentID == "" {
		return pkgerr.InvalidArgument("accrual requires payment_id")
	}
	return appendLedgerEntry(ctx, r.pool, e)
}

func (r *LedgerRepo) AppendReversal(ctx context.Context, e *domain.LedgerEntry) error {
	if e.EntryType != domain.LedgerReversal {
		return pkgerr.InvalidArgument("AppendReversal requires entry_type=reversal")
	}
	if e.AmountKopecks >= 0 {
		return pkgerr.InvalidArgument("reversal amount must be negative")
	}
	if e.PaymentID == "" || e.ReversesEntryID == 0 {
		return pkgerr.InvalidArgument("reversal requires payment_id and reverses_entry_id")
	}
	// Wrap in a TX and acquire the partner advisory lock so this refund-reversal
	// serialises against PayoutRepo.CreatePendingWithLedger.  Without the lock a
	// refund could commit between the scheduler's balance read and its payout
	// insert, letting the payout drain a balance the refund has just shrunk.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reversal begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := acquirePartnerLock(ctx, tx, e.PartnerType, e.PartnerID); err != nil {
		return fmt.Errorf("reversal partner lock: %w", err)
	}
	if err := appendLedgerEntry(ctx, tx, e); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("reversal commit: %w", err)
	}
	return nil
}

// ledgerExecutor is the narrow surface needed to INSERT one ledger row.
// Both *pgxpool.Pool and pgx.Tx satisfy it, so callers can append within
// their own transaction (PayoutRepo) or directly against the pool
// (AppendAccrual/AppendReversal) without exposing a private LedgerRepo method.
type ledgerExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// appendLedgerEntry inserts one row into partner_ledger.
//
// Lives at package scope (no LedgerRepo receiver) because it is also called
// by PayoutRepo inside its own transaction. Making it a method would force
// PayoutRepo to either construct a fake LedgerRepo or thread a LedgerRepo
// dependency just to reach this one helper.
//
// 23505 (unique_violation) from any partial unique index (accrual per payment,
// reversal per payment, payout per payout_id) is translated to AlreadyExists
// so callers can absorb duplicate webhooks without retry logic.
func appendLedgerEntry(ctx context.Context, exec ledgerExecutor, e *domain.LedgerEntry) error {
	const q = `
		INSERT INTO partner_ledger (
			partner_type, partner_id, entry_type, amount_kopecks,
			payment_id, payout_id, reverses_entry_id,
			available_at, reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`

	row := exec.QueryRow(ctx, q,
		string(e.PartnerType), e.PartnerID, string(e.EntryType), e.AmountKopecks,
		nullableString(e.PaymentID), nullableString(e.PayoutID),
		nullableInt64(e.ReversesEntryID),
		nullableTime(e.AvailableAt),
		nullableString(e.Reason),
	)
	if err := row.Scan(&e.ID, &e.CreatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return pkgerr.AlreadyExists("ledger entry already exists")
		}
		return fmt.Errorf("ledger insert: %w", err)
	}
	return nil
}

// Balance returns total, available, and held amounts in one round-trip.
// Available excludes accruals whose available_at is still in the future.
// Held is the difference — exactly the sum of accruals still in their
// hold window.
func (r *LedgerRepo) Balance(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PartnerBalance, error) {
	const q = `
		SELECT
			COALESCE(SUM(amount_kopecks), 0) AS total,
			COALESCE(SUM(CASE
				WHEN entry_type <> 'accrual' THEN amount_kopecks
				WHEN available_at IS NOT NULL AND available_at <= now() THEN amount_kopecks
				ELSE 0
			END), 0) AS available,
			MAX(created_at) AS last_entry_at
		FROM partner_ledger
		WHERE partner_type = $1 AND partner_id = $2`

	b := &domain.PartnerBalance{PartnerType: partnerType, PartnerID: partnerID}
	var lastEntryAt *time.Time
	if err := r.pool.QueryRow(ctx, q, string(partnerType), partnerID).Scan(
		&b.TotalKopecks, &b.AvailableKopecks, &lastEntryAt,
	); err != nil {
		return nil, fmt.Errorf("ledger balance: %w", err)
	}
	b.HeldKopecks = b.TotalKopecks - b.AvailableKopecks
	if lastEntryAt != nil {
		b.LastEntryAt = *lastEntryAt
	}
	return b, nil
}

// FindAccrualByPayment locates the accrual row recorded for paymentID.
// The partial unique index on (payment_id) WHERE entry_type='accrual'
// guarantees at most one match.
func (r *LedgerRepo) FindAccrualByPayment(ctx context.Context, paymentID string) (*domain.LedgerEntry, error) {
	const q = `
		SELECT id, partner_type, partner_id, entry_type, amount_kopecks,
		       COALESCE(payment_id::text, ''), COALESCE(payout_id::text, ''),
		       COALESCE(reverses_entry_id, 0),
		       available_at, COALESCE(reason, ''), created_at
		FROM partner_ledger
		WHERE payment_id = $1 AND entry_type = 'accrual'`

	e := &domain.LedgerEntry{}
	var partnerType, entryType string
	var availableAt *time.Time
	err := r.pool.QueryRow(ctx, q, paymentID).Scan(
		&e.ID, &partnerType, &e.PartnerID, &entryType, &e.AmountKopecks,
		&e.PaymentID, &e.PayoutID, &e.ReversesEntryID,
		&availableAt, &e.Reason, &e.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("accrual not found for payment")
	}
	if err != nil {
		return nil, fmt.Errorf("ledger find accrual: %w", err)
	}
	e.PartnerType = domain.PartnerType(partnerType)
	e.EntryType = domain.LedgerEntryType(entryType)
	if availableAt != nil {
		e.AvailableAt = *availableAt
	}
	return e, nil
}

func (r *LedgerRepo) List(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.LedgerEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	const q = `
		SELECT id, partner_type, partner_id, entry_type, amount_kopecks,
		       COALESCE(payment_id::text, ''), COALESCE(payout_id::text, ''),
		       COALESCE(reverses_entry_id, 0),
		       available_at, COALESCE(reason, ''), created_at
		FROM partner_ledger
		WHERE partner_type = $1 AND partner_id = $2
		ORDER BY id DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.pool.Query(ctx, q, string(partnerType), partnerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ledger list query: %w", err)
	}
	defer rows.Close()

	out := make([]domain.LedgerEntry, 0, limit)
	for rows.Next() {
		var e domain.LedgerEntry
		var partnerType, entryType string
		var availableAt *time.Time
		if err := rows.Scan(
			&e.ID, &partnerType, &e.PartnerID, &entryType, &e.AmountKopecks,
			&e.PaymentID, &e.PayoutID, &e.ReversesEntryID,
			&availableAt, &e.Reason, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("ledger list scan: %w", err)
		}
		e.PartnerType = domain.PartnerType(partnerType)
		e.EntryType = domain.LedgerEntryType(entryType)
		if availableAt != nil {
			e.AvailableAt = *availableAt
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger list rows: %w", err)
	}
	return out, nil
}

// PartnersWithAvailableBalance returns up to limit partners whose available
// balance is at least minKopecks. The query aggregates the ledger server-side
// using the same available-amount expression as Balance(), then filters by HAVING.
//
// This is read-only and intended for the payout scheduler's enumeration phase;
// the scheduler must then re-check the balance inside its own transaction
// before creating a payout — between this read and the write another payout
// may have already drained the partner.
func (r *LedgerRepo) PartnersWithAvailableBalance(ctx context.Context, minKopecks int64, limit int) ([]domain.PartnerRef, error) {
	if minKopecks <= 0 {
		return nil, pkgerr.InvalidArgument("minKopecks must be positive")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	const q = `
		SELECT partner_type, partner_id
		FROM partner_ledger
		GROUP BY partner_type, partner_id
		HAVING COALESCE(SUM(CASE
			WHEN entry_type <> 'accrual' THEN amount_kopecks
			WHEN available_at IS NOT NULL AND available_at <= now() THEN amount_kopecks
			ELSE 0
		END), 0) >= $1
		ORDER BY partner_type, partner_id
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, minKopecks, limit)
	if err != nil {
		return nil, fmt.Errorf("partners available query: %w", err)
	}
	defer rows.Close()

	out := make([]domain.PartnerRef, 0, limit)
	for rows.Next() {
		var pt string
		var pid string
		if err := rows.Scan(&pt, &pid); err != nil {
			return nil, fmt.Errorf("partners available scan: %w", err)
		}
		out = append(out, domain.PartnerRef{PartnerType: domain.PartnerType(pt), PartnerID: pid})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("partners available rows: %w", err)
	}
	return out, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
