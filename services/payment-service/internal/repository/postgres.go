package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

type PaymentRepo struct {
	pool *pgxpool.Pool
}

func NewPaymentRepo(pool *pgxpool.Pool) *PaymentRepo {
	return &PaymentRepo{pool: pool}
}

// CreateIdempotent inserts a new payment row only when no row with the same
// idempotency_key exists.  ON CONFLICT DO NOTHING means the INSERT is skipped
// silently when a duplicate key is detected; RETURNING returns nothing for the
// skipped row, causing pgx to surface pgx.ErrNoRows — which we translate to
// isNew=false so the caller can fetch the existing record.
//
// This is the correct DB-level idempotency primitive: the UNIQUE constraint on
// idempotency_key is the single authoritative lock, eliminating the Redis SETNX
// race where two concurrent requests could both see "key not found" before either
// had written to the DB.
func (r *PaymentRepo) CreateIdempotent(ctx context.Context, p *domain.Payment) (bool, error) {
	const q = `
		INSERT INTO payments (booking_id, amount, status, provider_id, payment_url, idempotency_key,
			platform_fee_kopecks, counterparty_net_kopecks, counterparty_type, counterparty_id,
			provider_seller_account_id, provider_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, created_at, updated_at`
	providerName := p.ProviderName
	if providerName == "" {
		providerName = "yookassa"
	}
	err := r.pool.QueryRow(ctx, q,
		p.BookingID, p.Amount, p.Status.String(),
		p.ProviderID, p.PaymentURL, p.IdempotencyKey,
		p.PlatformFeeKopecks, p.CounterpartyNetKopecks, p.CounterpartyType, p.CounterpartyID,
		p.ProviderSellerAccountID, providerName,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Conflict: a row with this idempotency_key already exists.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// UpdateProviderInfo sets the external provider reference on an existing payment
// row after the provider call succeeds.  Called in the second phase of payment
// creation so that future retries with the same idempotency_key can short-circuit
// without calling the provider again.
func (r *PaymentRepo) UpdateProviderInfo(ctx context.Context, id, providerID, paymentURL string) error {
	const q = `UPDATE payments SET provider_id = $2, payment_url = $3, updated_at = now() WHERE id = $1`
	ct, err := r.pool.Exec(ctx, q, id, providerID, paymentURL)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pkgerr.NotFound("payment not found")
	}
	return nil
}

func (r *PaymentRepo) GetByID(ctx context.Context, id string) (*domain.Payment, error) {
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       COALESCE(platform_fee_kopecks, 0), COALESCE(counterparty_net_kopecks, 0),
		       COALESCE(counterparty_type, ''), COALESCE(counterparty_id, ''),
		       COALESCE(provider_seller_account_id, ''), COALESCE(provider_name, 'yookassa'),
		       created_at, updated_at
		FROM payments WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

func (r *PaymentRepo) GetByBookingID(ctx context.Context, bookingID string) (*domain.Payment, error) {
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       COALESCE(platform_fee_kopecks, 0), COALESCE(counterparty_net_kopecks, 0),
		       COALESCE(counterparty_type, ''), COALESCE(counterparty_id, ''),
		       COALESCE(provider_seller_account_id, ''), COALESCE(provider_name, 'yookassa'),
		       created_at, updated_at
		FROM payments WHERE booking_id = $1`
	return r.scanOne(ctx, q, bookingID)
}

func (r *PaymentRepo) GetByProviderID(ctx context.Context, providerID string) (*domain.Payment, error) {
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       COALESCE(platform_fee_kopecks, 0), COALESCE(counterparty_net_kopecks, 0),
		       COALESCE(counterparty_type, ''), COALESCE(counterparty_id, ''),
		       COALESCE(provider_seller_account_id, ''), COALESCE(provider_name, 'yookassa'),
		       created_at, updated_at
		FROM payments WHERE provider_id = $1`
	return r.scanOne(ctx, q, providerID)
}

func (r *PaymentRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       COALESCE(platform_fee_kopecks, 0), COALESCE(counterparty_net_kopecks, 0),
		       COALESCE(counterparty_type, ''), COALESCE(counterparty_id, ''),
		       COALESCE(provider_seller_account_id, ''), COALESCE(provider_name, 'yookassa'),
		       created_at, updated_at
		FROM payments WHERE idempotency_key = $1`
	return r.scanOne(ctx, q, key)
}

// UpdateStatus transitions a payment to the given status only when it is not
// already in a terminal state.
//
// The WHERE clause mirrors domain.PaymentStatus.IsTerminal(): status NOT IN
// ('succeeded', 'cancelled', 'refunded').  This is the idempotency guard
// against duplicate webhook delivery: if the same event arrives a second time
// the row simply won't be updated and RowsAffected will be 0, returned as
// updated=false.  The caller must treat updated=false as a no-op and skip event
// publication.
//
// A genuine "not found" (no row for the given id) is also reported as
// updated=false with a NotFound sentinel so the caller can distinguish the two
// cases if needed.
func (r *PaymentRepo) UpdateStatus(ctx context.Context, id string, newStatus domain.PaymentStatus, providerID string) (bool, error) {
	const q = `
		UPDATE payments
		   SET status = $2, provider_id = $3, updated_at = now()
		 WHERE id = $1
		   AND status NOT IN ('succeeded', 'cancelled', 'refunded')`
	ct, err := r.pool.Exec(ctx, q, id, newStatus.String(), providerID)
	if err != nil {
		return false, err
	}
	if ct.RowsAffected() == 0 {
		exists, chkErr := r.paymentExists(ctx, id)
		if chkErr != nil {
			return false, chkErr
		}
		if !exists {
			return false, pkgerr.NotFound("payment not found")
		}
		// Already terminal — duplicate delivery, safe to skip.
		return false, nil
	}
	return true, nil
}

func (r *PaymentRepo) paymentExists(ctx context.Context, id string) (bool, error) {
	const q = `SELECT 1 FROM payments WHERE id = $1`
	var dummy int
	err := r.pool.QueryRow(ctx, q, id).Scan(&dummy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// DriveProviderWithLock serialises concurrent crash-recovery attempts for the
// same idempotency_key using a PostgreSQL session-level advisory lock.
//
// The lock key is derived via hashtext(idempotencyKey) — a deterministic int4
// built into PostgreSQL, collision-resistant for our cardinality of keys.
// pg_advisory_xact_lock blocks until the lock is available and releases it
// automatically when the surrounding transaction commits or rolls back.
//
// Flow:
//  1. Begin transaction.
//  2. Acquire pg_advisory_xact_lock(hashtext(idempotencyKey)).
//  3. Re-read the payments row.
//     a. ProviderID already set → commit (no-op) and return the row.
//        The first requester finished while we were waiting — no provider call.
//     b. ProviderID empty → call fn(p).
//        On success: write providerID+paymentURL, commit, return updated row.
//        On fn error: rollback, return error so caller can propagate.
func (r *PaymentRepo) DriveProviderWithLock(
	ctx context.Context,
	idempotencyKey string,
	fn func(p *domain.Payment) (providerID, paymentURL string, err error),
) (*domain.Payment, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("DriveProviderWithLock begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Acquire advisory lock scoped to this transaction.
	// hashtext() is an internal PostgreSQL function that returns int4 — safe to
	// use as an advisory lock key for string-keyed resources.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, idempotencyKey); err != nil {
		return nil, fmt.Errorf("DriveProviderWithLock acquire lock: %w", err)
	}

	// Re-read the row now that we hold the lock.
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       COALESCE(platform_fee_kopecks, 0), COALESCE(counterparty_net_kopecks, 0),
		       COALESCE(counterparty_type, ''), COALESCE(counterparty_id, ''),
		       COALESCE(provider_seller_account_id, ''), COALESCE(provider_name, 'yookassa'),
		       created_at, updated_at
		FROM payments WHERE idempotency_key = $1`

	p := &domain.Payment{}
	var rawStatus string
	scanErr := tx.QueryRow(ctx, q, idempotencyKey).Scan(
		&p.ID, &p.BookingID, &p.Amount, &rawStatus,
		&p.ProviderID, &p.PaymentURL, &p.IdempotencyKey,
		&p.PlatformFeeKopecks, &p.CounterpartyNetKopecks, &p.CounterpartyType, &p.CounterpartyID,
		&p.ProviderSellerAccountID, &p.ProviderName,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(scanErr, pgx.ErrNoRows) {
		err = pkgerr.NotFound("payment not found for idempotency key")
		return nil, err
	}
	if scanErr != nil {
		err = scanErr
		return nil, err
	}
	p.Status = domain.PaymentStatus(rawStatus)

	// If the first requester already wrote the provider reference, return it
	// without touching the provider — exactly-once guarantee satisfied.
	if p.ProviderID != "" {
		err = tx.Commit(ctx)
		return p, err
	}

	// We are the designated driver.  Call the provider inside the lock window.
	providerID, paymentURL, fnErr := fn(p)
	if fnErr != nil {
		err = fnErr // triggers deferred Rollback
		return nil, err
	}

	// Persist the provider reference inside the same transaction.
	const uq = `UPDATE payments SET provider_id = $2, payment_url = $3, updated_at = now() WHERE id = $1`
	ct, uErr := tx.Exec(ctx, uq, p.ID, providerID, paymentURL)
	if uErr != nil {
		err = uErr
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		err = pkgerr.NotFound("payment row disappeared during lock window")
		return nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("DriveProviderWithLock commit: %w", err)
	}

	p.ProviderID = providerID
	p.PaymentURL = paymentURL
	return p, nil
}

// scanOne executes q with args and scans a single payments row.
// The status column is a raw string in the DB; it is cast to domain.PaymentStatus
// (a named string type) directly — no lookup table needed since the DB values
// are defined to match the constant literals in domain exactly.
func (r *PaymentRepo) scanOne(ctx context.Context, q string, args ...any) (*domain.Payment, error) {
	p := &domain.Payment{}
	var rawStatus string
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&p.ID, &p.BookingID, &p.Amount, &rawStatus,
		&p.ProviderID, &p.PaymentURL, &p.IdempotencyKey,
		&p.PlatformFeeKopecks, &p.CounterpartyNetKopecks, &p.CounterpartyType, &p.CounterpartyID,
		&p.ProviderSellerAccountID, &p.ProviderName,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("payment not found")
	}
	if err != nil {
		return nil, err
	}
	p.Status = domain.PaymentStatus(rawStatus)
	return p, nil
}
