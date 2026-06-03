package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

// nullableTime maps Go's time.Time zero value to a SQL NULL.
// pgx serialises a non-nil *time.Time as TIMESTAMPTZ and nil as NULL,
// so this is the idiomatic way to keep "unset" columns NULL rather
// than inserting the year-0001 sentinel that a zero time.Time would yield.
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

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
			provider_name, service_at)
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
		providerName, nullableTime(p.ServiceAt),
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

// paymentSelectCols is the column list shared by every payments SELECT.
// Keeping a single copy guarantees the scan order in scanOne stays consistent
// across all lookup methods — adding a new column requires touching one place.
const paymentSelectCols = `
	id, booking_id, amount, status, COALESCE(provider_id, ''),
	COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
	COALESCE(platform_fee_kopecks, 0), COALESCE(counterparty_net_kopecks, 0),
	COALESCE(counterparty_type, ''), COALESCE(counterparty_id, ''),
	COALESCE(provider_name, 'yookassa'), service_at,
	created_at, updated_at`

func (r *PaymentRepo) GetByID(ctx context.Context, id string) (*domain.Payment, error) {
	return r.scanOne(ctx, `SELECT `+paymentSelectCols+` FROM payments WHERE id = $1`, id)
}

func (r *PaymentRepo) GetByBookingID(ctx context.Context, bookingID string) (*domain.Payment, error) {
	return r.scanOne(ctx, `SELECT `+paymentSelectCols+` FROM payments WHERE booking_id = $1`, bookingID)
}

func (r *PaymentRepo) GetByProviderID(ctx context.Context, providerID string) (*domain.Payment, error) {
	return r.scanOne(ctx, `SELECT `+paymentSelectCols+` FROM payments WHERE provider_id = $1`, providerID)
}

func (r *PaymentRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	return r.scanOne(ctx, `SELECT `+paymentSelectCols+` FROM payments WHERE idempotency_key = $1`, key)
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
//     The first requester finished while we were waiting — no provider call.
//     b. ProviderID empty → call fn(p).
//     On success: write providerID+paymentURL, commit, return updated row.
//     On fn error: rollback, return error so caller can propagate.
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
	q := `SELECT ` + paymentSelectCols + ` FROM payments WHERE idempotency_key = $1`

	p := &domain.Payment{}
	var rawStatus string
	var serviceAt *time.Time
	scanErr := tx.QueryRow(ctx, q, idempotencyKey).Scan(
		&p.ID, &p.BookingID, &p.Amount, &rawStatus,
		&p.ProviderID, &p.PaymentURL, &p.IdempotencyKey,
		&p.PlatformFeeKopecks, &p.CounterpartyNetKopecks, &p.CounterpartyType, &p.CounterpartyID,
		&p.ProviderName, &serviceAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if serviceAt != nil {
		p.ServiceAt = *serviceAt
	}
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
	var serviceAt *time.Time
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&p.ID, &p.BookingID, &p.Amount, &rawStatus,
		&p.ProviderID, &p.PaymentURL, &p.IdempotencyKey,
		&p.PlatformFeeKopecks, &p.CounterpartyNetKopecks, &p.CounterpartyType, &p.CounterpartyID,
		&p.ProviderName, &serviceAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("payment not found")
	}
	if err != nil {
		return nil, err
	}
	p.Status = domain.PaymentStatus(rawStatus)
	if serviceAt != nil {
		p.ServiceAt = *serviceAt
	}
	return p, nil
}
