package repository

import (
	"context"
	"errors"

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

func (r *PaymentRepo) Create(ctx context.Context, p *domain.Payment) error {
	const q = `
		INSERT INTO payments (booking_id, amount, status, provider_id, payment_url, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, q,
		p.BookingID, p.Amount, p.Status,
		p.ProviderID, p.PaymentURL, p.IdempotencyKey,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (r *PaymentRepo) GetByID(ctx context.Context, id string) (*domain.Payment, error) {
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       created_at, updated_at
		FROM payments WHERE id = $1`
	return r.scanOne(ctx, q, id)
}

func (r *PaymentRepo) GetByBookingID(ctx context.Context, bookingID string) (*domain.Payment, error) {
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       created_at, updated_at
		FROM payments WHERE booking_id = $1`
	return r.scanOne(ctx, q, bookingID)
}

func (r *PaymentRepo) GetByProviderID(ctx context.Context, providerID string) (*domain.Payment, error) {
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       created_at, updated_at
		FROM payments WHERE provider_id = $1`
	return r.scanOne(ctx, q, providerID)
}

func (r *PaymentRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	const q = `
		SELECT id, booking_id, amount, status, COALESCE(provider_id, ''),
		       COALESCE(payment_url, ''), COALESCE(idempotency_key, ''),
		       created_at, updated_at
		FROM payments WHERE idempotency_key = $1`
	return r.scanOne(ctx, q, key)
}

func (r *PaymentRepo) UpdateStatus(ctx context.Context, id, status, providerID string) error {
	const q = `UPDATE payments SET status = $2, provider_id = $3, updated_at = now() WHERE id = $1`
	ct, err := r.pool.Exec(ctx, q, id, status, providerID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pkgerr.NotFound("payment not found")
	}
	return nil
}

func (r *PaymentRepo) scanOne(ctx context.Context, q string, args ...any) (*domain.Payment, error) {
	p := &domain.Payment{}
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&p.ID, &p.BookingID, &p.Amount, &p.Status,
		&p.ProviderID, &p.PaymentURL, &p.IdempotencyKey,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("payment not found")
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}
