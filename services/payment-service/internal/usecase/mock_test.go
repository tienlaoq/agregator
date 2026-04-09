package usecase

import (
	"context"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

type mockPaymentRepo struct {
	CreateFunc              func(ctx context.Context, p *domain.Payment) error
	GetByIDFunc             func(ctx context.Context, id string) (*domain.Payment, error)
	GetByBookingIDFunc      func(ctx context.Context, bookingID string) (*domain.Payment, error)
	GetByProviderIDFunc     func(ctx context.Context, providerID string) (*domain.Payment, error)
	GetByIdempotencyKeyFunc func(ctx context.Context, key string) (*domain.Payment, error)
	UpdateStatusFunc        func(ctx context.Context, id, status, providerID string) error
}

func (m *mockPaymentRepo) Create(ctx context.Context, p *domain.Payment) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, p)
	}
	return nil
}

func (m *mockPaymentRepo) GetByID(ctx context.Context, id string) (*domain.Payment, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockPaymentRepo) GetByBookingID(ctx context.Context, bookingID string) (*domain.Payment, error) {
	if m.GetByBookingIDFunc != nil {
		return m.GetByBookingIDFunc(ctx, bookingID)
	}
	return nil, nil
}

func (m *mockPaymentRepo) GetByProviderID(ctx context.Context, providerID string) (*domain.Payment, error) {
	if m.GetByProviderIDFunc != nil {
		return m.GetByProviderIDFunc(ctx, providerID)
	}
	return nil, nil
}

func (m *mockPaymentRepo) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	if m.GetByIdempotencyKeyFunc != nil {
		return m.GetByIdempotencyKeyFunc(ctx, key)
	}
	return nil, nil
}

func (m *mockPaymentRepo) UpdateStatus(ctx context.Context, id, status, providerID string) error {
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, id, status, providerID)
	}
	return nil
}

type mockEventPublisher struct {
	PublishPaymentCompletedFunc func(ctx context.Context, p *domain.Payment) error
	PublishPaymentFailedFunc    func(ctx context.Context, p *domain.Payment) error
}

func (m *mockEventPublisher) PublishPaymentCompleted(ctx context.Context, p *domain.Payment) error {
	if m.PublishPaymentCompletedFunc != nil {
		return m.PublishPaymentCompletedFunc(ctx, p)
	}
	return nil
}

func (m *mockEventPublisher) PublishPaymentFailed(ctx context.Context, p *domain.Payment) error {
	if m.PublishPaymentFailedFunc != nil {
		return m.PublishPaymentFailedFunc(ctx, p)
	}
	return nil
}
