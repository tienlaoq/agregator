package usecase

import (
	"context"
	"net/http"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// ── mockPaymentRepo ───────────────────────────────────────────────────────────

type mockPaymentRepo struct {
	CreateIdempotentFunc          func(ctx context.Context, p *domain.Payment) (bool, error)
	UpdateProviderInfoFunc        func(ctx context.Context, id, providerID, paymentURL string) error
	GetByIDFunc                   func(ctx context.Context, id string) (*domain.Payment, error)
	GetByBookingIDFunc            func(ctx context.Context, bookingID string) (*domain.Payment, error)
	GetByProviderIDFunc           func(ctx context.Context, providerID string) (*domain.Payment, error)
	GetByIdempotencyKeyFunc       func(ctx context.Context, key string) (*domain.Payment, error)
	UpdateStatusFunc              func(ctx context.Context, id string, status domain.PaymentStatus, providerID string) (bool, error)
	UpdateStatusWithOutboxFunc    func(ctx context.Context, id string, status domain.PaymentStatus, providerID string, event *domain.OutboxEvent) (bool, error)
	DriveProviderWithLockFunc     func(ctx context.Context, idempotencyKey string, fn func(p *domain.Payment) (string, string, error)) (*domain.Payment, error)
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

func (m *mockPaymentRepo) UpdateStatus(ctx context.Context, id string, st domain.PaymentStatus, providerID string) (bool, error) {
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, id, st, providerID)
	}
	// Default false: tests must explicitly set UpdateStatusFunc to assert that
	// a status transition occurred.  A silent true would mask missing stubs.
	return false, nil
}

func (m *mockPaymentRepo) UpdateStatusWithOutbox(ctx context.Context, id string, st domain.PaymentStatus, providerID string, event *domain.OutboxEvent) (bool, error) {
	if m.UpdateStatusWithOutboxFunc != nil {
		return m.UpdateStatusWithOutboxFunc(ctx, id, st, providerID, event)
	}
	// Default false: tests must explicitly set UpdateStatusWithOutboxFunc and
	// return true to signal that the row was updated.  Relying on the default
	// would let a test pass even if the usecase skipped the update entirely
	// (e.g. because IsTerminal filtered it out or the wrong branch was taken).
	return false, nil
}

func (m *mockPaymentRepo) DriveProviderWithLock(ctx context.Context, idempotencyKey string, fn func(p *domain.Payment) (string, string, error)) (*domain.Payment, error) {
	if m.DriveProviderWithLockFunc != nil {
		return m.DriveProviderWithLockFunc(ctx, idempotencyKey, fn)
	}
	// Default: call fn with a zero Payment so tests that don't set the func
	// still exercise the happy path without panicking.
	p := &domain.Payment{IdempotencyKey: idempotencyKey}
	providerID, paymentURL, err := fn(p)
	if err != nil {
		return nil, err
	}
	p.ProviderID = providerID
	p.PaymentURL = paymentURL
	return p, nil
}

// ── mockOutboxRepo ────────────────────────────────────────────────────────────

// mockOutboxRepo is a test double for domain.OutboxRepository.
// Tests that want to assert on outbox writes should supply AppendTxFunc.
// RelayBatch is not exercised by usecase tests (it is a repository concern);
// the default no-op keeps the interface satisfied without noise in test output.
type mockOutboxRepo struct {
	AppendTxFunc    func(ctx context.Context, tx any, event *domain.OutboxEvent) error
	RelayBatchFunc  func(ctx context.Context, limit int, publish func(*domain.OutboxEvent) error) (int, error)
	MarkFailedFunc  func(ctx context.Context, id int64, errMsg string) error
}

func (m *mockOutboxRepo) AppendTx(ctx context.Context, tx any, event *domain.OutboxEvent) error {
	if m.AppendTxFunc != nil {
		return m.AppendTxFunc(ctx, tx, event)
	}
	return nil
}

func (m *mockOutboxRepo) RelayBatch(ctx context.Context, limit int, publish func(*domain.OutboxEvent) error) (int, error) {
	if m.RelayBatchFunc != nil {
		return m.RelayBatchFunc(ctx, limit, publish)
	}
	return 0, nil
}

func (m *mockOutboxRepo) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	if m.MarkFailedFunc != nil {
		return m.MarkFailedFunc(ctx, id, errMsg)
	}
	return nil
}

// ── mockPaymentProvider ───────────────────────────────────────────────────────

// mockPaymentProvider is a test double for provider.PaymentProvider.
// Individual methods can be overridden per test via the Func fields.
type mockPaymentProvider struct {
	CreatePaymentFunc   func(ctx context.Context, req provider.CreateRequest) (*provider.Result, error)
	CaptureFunc         func(ctx context.Context, providerPaymentID, idempotencyKey string) error
	RefundFunc          func(ctx context.Context, providerPaymentID string, amountKopecks int64, idempotencyKey string) error
	ParseWebhookFunc    func(ctx context.Context, rawBody []byte, headers http.Header) (*domain.WebhookEvent, error)
	VerifySignatureFunc func(ctx context.Context, rawBody []byte, headers http.Header) error
	MockMode            bool
}

func (m *mockPaymentProvider) CreatePayment(ctx context.Context, req provider.CreateRequest) (*provider.Result, error) {
	if m.CreatePaymentFunc != nil {
		return m.CreatePaymentFunc(ctx, req)
	}
	return &provider.Result{
		ProviderPaymentID: "mock-pay",
		ConfirmationURL:   "https://mock.yookassa.ru/pay/mock-pay",
	}, nil
}

func (m *mockPaymentProvider) Capture(ctx context.Context, providerPaymentID, idempotencyKey string) error {
	if m.CaptureFunc != nil {
		return m.CaptureFunc(ctx, providerPaymentID, idempotencyKey)
	}
	return nil
}

func (m *mockPaymentProvider) Refund(ctx context.Context, providerPaymentID string, amountKopecks int64, idempotencyKey string) error {
	if m.RefundFunc != nil {
		return m.RefundFunc(ctx, providerPaymentID, amountKopecks, idempotencyKey)
	}
	return nil
}

func (m *mockPaymentProvider) ParseWebhook(ctx context.Context, rawBody []byte, headers http.Header) (*domain.WebhookEvent, error) {
	if m.ParseWebhookFunc != nil {
		return m.ParseWebhookFunc(ctx, rawBody, headers)
	}
	// Default: return a succeeded event with an empty provider ID — tests that
	// need a specific result must supply ParseWebhookFunc.
	return &domain.WebhookEvent{Status: domain.StatusSucceeded}, nil
}

func (m *mockPaymentProvider) VerifySignature(ctx context.Context, rawBody []byte, headers http.Header) error {
	if m.VerifySignatureFunc != nil {
		return m.VerifySignatureFunc(ctx, rawBody, headers)
	}
	return nil
}

func (m *mockPaymentProvider) IsMockMode() bool { return m.MockMode }
