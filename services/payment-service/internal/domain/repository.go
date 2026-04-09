package domain

import "context"

type PaymentRepository interface {
	Create(ctx context.Context, p *Payment) error
	GetByID(ctx context.Context, id string) (*Payment, error)
	GetByBookingID(ctx context.Context, bookingID string) (*Payment, error)
	GetByProviderID(ctx context.Context, providerID string) (*Payment, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Payment, error)
	UpdateStatus(ctx context.Context, id, status, providerID string) error
}
