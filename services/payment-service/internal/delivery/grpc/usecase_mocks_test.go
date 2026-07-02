package grpc

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// These mocks mirror internal/usecase/mock_test.go so the delivery layer can be
// driven through real usecases wired with test doubles. Only the methods a
// handler path touches need overriding; defaults keep the rest quiet.

type mockPaymentRepo struct {
	CreateIdempotentFunc       func(ctx context.Context, p *domain.Payment) (bool, error)
	UpdateProviderInfoFunc     func(ctx context.Context, id, providerID, paymentURL string) error
	GetByIDFunc                func(ctx context.Context, id string) (*domain.Payment, error)
	GetByBookingIDFunc         func(ctx context.Context, bookingID string) (*domain.Payment, error)
	GetByProviderIDFunc        func(ctx context.Context, providerID string) (*domain.Payment, error)
	GetByIdempotencyKeyFunc    func(ctx context.Context, key string) (*domain.Payment, error)
	UpdateStatusWithOutboxFunc func(ctx context.Context, id string, status domain.PaymentStatus, providerID string, event *domain.OutboxEvent) (bool, error)
	DriveProviderWithLockFunc  func(ctx context.Context, idempotencyKey string, fn func(p *domain.Payment) (string, string, error)) (*domain.Payment, error)
}

func (m *mockPaymentRepo) CreateIdempotent(ctx context.Context, p *domain.Payment) (bool, error) {
	if m.CreateIdempotentFunc != nil {
		return m.CreateIdempotentFunc(ctx, p)
	}
	return true, nil
}
func (m *mockPaymentRepo) UpdateProviderInfo(ctx context.Context, id, providerID, paymentURL string) error {
	if m.UpdateProviderInfoFunc != nil {
		return m.UpdateProviderInfoFunc(ctx, id, providerID, paymentURL)
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
func (m *mockPaymentRepo) UpdateStatusWithOutbox(ctx context.Context, id string, st domain.PaymentStatus, providerID string, event *domain.OutboxEvent) (bool, error) {
	if m.UpdateStatusWithOutboxFunc != nil {
		return m.UpdateStatusWithOutboxFunc(ctx, id, st, providerID, event)
	}
	return false, nil
}
func (m *mockPaymentRepo) DriveProviderWithLock(ctx context.Context, idempotencyKey string, fn func(p *domain.Payment) (string, string, error)) (*domain.Payment, error) {
	if m.DriveProviderWithLockFunc != nil {
		return m.DriveProviderWithLockFunc(ctx, idempotencyKey, fn)
	}
	p := &domain.Payment{IdempotencyKey: idempotencyKey}
	providerID, paymentURL, err := fn(p)
	if err != nil {
		return nil, err
	}
	p.ProviderID = providerID
	p.PaymentURL = paymentURL
	return p, nil
}

type mockOutboxRepo struct{}

func (mockOutboxRepo) RelayBatch(context.Context, int, func(*domain.OutboxEvent) error) (int, error) {
	return 0, nil
}
func (mockOutboxRepo) MarkFailed(context.Context, int64, string) error { return nil }

type mockLedgerRepo struct {
	AppendAccrualFunc        func(ctx context.Context, e *domain.LedgerEntry) error
	AppendReversalFunc       func(ctx context.Context, e *domain.LedgerEntry) error
	FindAccrualByPaymentFunc func(ctx context.Context, paymentID string) (*domain.LedgerEntry, error)
	BalanceFunc              func(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PartnerBalance, error)
	ListFunc                 func(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.LedgerEntry, error)
}

func (m *mockLedgerRepo) AppendAccrual(ctx context.Context, e *domain.LedgerEntry) error {
	if m.AppendAccrualFunc != nil {
		return m.AppendAccrualFunc(ctx, e)
	}
	return nil
}
func (m *mockLedgerRepo) AppendReversal(ctx context.Context, e *domain.LedgerEntry) error {
	if m.AppendReversalFunc != nil {
		return m.AppendReversalFunc(ctx, e)
	}
	return nil
}
func (m *mockLedgerRepo) FindAccrualByPayment(ctx context.Context, paymentID string) (*domain.LedgerEntry, error) {
	if m.FindAccrualByPaymentFunc != nil {
		return m.FindAccrualByPaymentFunc(ctx, paymentID)
	}
	return nil, nil
}
func (m *mockLedgerRepo) Balance(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PartnerBalance, error) {
	if m.BalanceFunc != nil {
		return m.BalanceFunc(ctx, partnerType, partnerID)
	}
	return &domain.PartnerBalance{PartnerType: partnerType, PartnerID: partnerID}, nil
}
func (m *mockLedgerRepo) List(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.LedgerEntry, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, partnerType, partnerID, limit, offset)
	}
	return nil, nil
}
func (m *mockLedgerRepo) PartnersWithAvailableBalance(context.Context, int64, int) ([]domain.PartnerRef, error) {
	return nil, nil
}

type mockPayoutMethodRepo struct {
	SaveFunc      func(ctx context.Context, m *domain.PayoutMethod) error
	GetActiveFunc func(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PayoutMethod, error)
}

func (m *mockPayoutMethodRepo) Save(ctx context.Context, pm *domain.PayoutMethod) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, pm)
	}
	if pm.ID == "" {
		pm.ID = "method-mock"
	}
	return nil
}
func (m *mockPayoutMethodRepo) GetActive(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PayoutMethod, error) {
	if m.GetActiveFunc != nil {
		return m.GetActiveFunc(ctx, partnerType, partnerID)
	}
	return nil, status.Error(codes.NotFound, "no active method")
}
func (m *mockPayoutMethodRepo) GetByID(context.Context, string) (*domain.PayoutMethod, error) {
	return nil, status.Error(codes.NotFound, "method not found")
}

type mockPayoutRepo struct {
	ListFunc func(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.Payout, error)
}

func (mockPayoutRepo) CreatePendingWithLedger(context.Context, *domain.Payout) error { return nil }
func (mockPayoutRepo) MarkProcessing(context.Context, string, string) error          { return nil }
func (mockPayoutRepo) MarkSucceeded(context.Context, string, time.Time) (bool, error) {
	return true, nil
}
func (mockPayoutRepo) MarkFailedWithReversal(context.Context, string, string, time.Time) (bool, error) {
	return true, nil
}
func (mockPayoutRepo) GetByID(context.Context, string) (*domain.Payout, error) {
	return nil, status.Error(codes.NotFound, "payout not found")
}
func (mockPayoutRepo) GetByProviderPayoutID(context.Context, string) (*domain.Payout, error) {
	return nil, status.Error(codes.NotFound, "payout not found")
}
func (m mockPayoutRepo) List(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.Payout, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, partnerType, partnerID, limit, offset)
	}
	return nil, nil
}
func (mockPayoutRepo) LastPayoutAt(context.Context, domain.PartnerType, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (mockPayoutRepo) ListPendingOlderThan(context.Context, time.Duration, int) ([]domain.Payout, error) {
	return nil, nil
}

type mockProvider struct {
	CreatePaymentFunc      func(ctx context.Context, req provider.CreateRequest) (*provider.Result, error)
	ParseWebhookFunc       func(ctx context.Context, rawBody []byte, h http.Header) (*domain.WebhookEvent, error)
	ParsePayoutWebhookFunc func(ctx context.Context, rawBody []byte, h http.Header) (*provider.PayoutWebhookEvent, error)
}

func (m *mockProvider) CreatePayment(ctx context.Context, req provider.CreateRequest) (*provider.Result, error) {
	if m.CreatePaymentFunc != nil {
		return m.CreatePaymentFunc(ctx, req)
	}
	return &provider.Result{ProviderPaymentID: "mock-pay", ConfirmationURL: "https://mock.local/pay/mock-pay"}, nil
}
func (m *mockProvider) Capture(context.Context, string, string) error              { return nil }
func (m *mockProvider) Refund(context.Context, string, int64, string) error        { return nil }
func (m *mockProvider) VerifySignature(context.Context, []byte, http.Header) error { return nil }
func (m *mockProvider) IsMockMode() bool                                           { return true }
func (m *mockProvider) CreatePayout(context.Context, provider.PayoutRequest) (*provider.PayoutResult, error) {
	return &provider.PayoutResult{ProviderPayoutID: "mock-payout", Status: provider.PayoutStatusSucceeded}, nil
}
func (m *mockProvider) ParseWebhook(ctx context.Context, rawBody []byte, h http.Header) (*domain.WebhookEvent, error) {
	if m.ParseWebhookFunc != nil {
		return m.ParseWebhookFunc(ctx, rawBody, h)
	}
	return &domain.WebhookEvent{Status: domain.StatusPending}, nil
}
func (m *mockProvider) ParsePayoutWebhook(ctx context.Context, rawBody []byte, h http.Header) (*provider.PayoutWebhookEvent, error) {
	if m.ParsePayoutWebhookFunc != nil {
		return m.ParsePayoutWebhookFunc(ctx, rawBody, h)
	}
	return nil, nil
}
