package usecase

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// ── mockPaymentRepo ───────────────────────────────────────────────────────────

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
// RelayBatch is not exercised by usecase tests (it is a repository concern);
// the default no-op keeps the interface satisfied without noise in test output.
type mockOutboxRepo struct {
	RelayBatchFunc func(ctx context.Context, limit int, publish func(*domain.OutboxEvent) error) (int, error)
	MarkFailedFunc func(ctx context.Context, id int64, errMsg string) error
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

// ── mockLedgerRepo ───────────────────────────────────────────────────────────

// mockLedgerRepo is a test double for domain.LedgerRepository.  Defaults to
// no-op success on every method so existing tests don't have to set up the
// ledger surface to exercise the payment flow.
type mockLedgerRepo struct {
	AppendAccrualFunc                func(ctx context.Context, e *domain.LedgerEntry) error
	AppendReversalFunc               func(ctx context.Context, e *domain.LedgerEntry) error
	FindAccrualByPaymentFunc         func(ctx context.Context, paymentID string) (*domain.LedgerEntry, error)
	BalanceFunc                      func(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PartnerBalance, error)
	ListFunc                         func(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.LedgerEntry, error)
	PartnersWithAvailableBalanceFunc func(ctx context.Context, minKopecks int64, limit int) ([]domain.PartnerRef, error)
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

func (m *mockLedgerRepo) PartnersWithAvailableBalance(ctx context.Context, minKopecks int64, limit int) ([]domain.PartnerRef, error) {
	if m.PartnersWithAvailableBalanceFunc != nil {
		return m.PartnersWithAvailableBalanceFunc(ctx, minKopecks, limit)
	}
	return nil, nil
}

// ── mockPaymentProvider ───────────────────────────────────────────────────────

// mockPaymentProvider is a test double for provider.PaymentProvider.
// Individual methods can be overridden per test via the Func fields.
type mockPaymentProvider struct {
	CreatePaymentFunc      func(ctx context.Context, req provider.CreateRequest) (*provider.Result, error)
	CaptureFunc            func(ctx context.Context, providerPaymentID, idempotencyKey string) error
	RefundFunc             func(ctx context.Context, providerPaymentID string, amountKopecks int64, idempotencyKey string) error
	CreatePayoutFunc       func(ctx context.Context, req provider.PayoutRequest) (*provider.PayoutResult, error)
	ParseWebhookFunc       func(ctx context.Context, rawBody []byte, headers http.Header) (*domain.WebhookEvent, error)
	ParsePayoutWebhookFunc func(ctx context.Context, rawBody []byte, headers http.Header) (*provider.PayoutWebhookEvent, error)
	VerifySignatureFunc    func(ctx context.Context, rawBody []byte, headers http.Header) error
	MockMode               bool
}

func (m *mockPaymentProvider) CreatePayment(ctx context.Context, req provider.CreateRequest) (*provider.Result, error) {
	if m.CreatePaymentFunc != nil {
		return m.CreatePaymentFunc(ctx, req)
	}
	return &provider.Result{
		ProviderPaymentID: "mock-pay",
		ConfirmationURL:   "https://mock.local/pay/mock-pay",
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

func (m *mockPaymentProvider) CreatePayout(ctx context.Context, req provider.PayoutRequest) (*provider.PayoutResult, error) {
	if m.CreatePayoutFunc != nil {
		return m.CreatePayoutFunc(ctx, req)
	}
	return &provider.PayoutResult{ProviderPayoutID: "mock-payout", Status: provider.PayoutStatusSucceeded}, nil
}

func (m *mockPaymentProvider) ParsePayoutWebhook(ctx context.Context, rawBody []byte, headers http.Header) (*provider.PayoutWebhookEvent, error) {
	if m.ParsePayoutWebhookFunc != nil {
		return m.ParsePayoutWebhookFunc(ctx, rawBody, headers)
	}
	// Default to "not a payout event" so existing tests don't get spurious
	// payout dispatch when they pass a payment webhook through HandleWebhook.
	return nil, nil
}

func (m *mockPaymentProvider) IsMockMode() bool { return m.MockMode }

// ── mockPayoutMethodRepo ───────────────────────────────────────────────────────

// mockPayoutMethodRepo is a test double for domain.PayoutMethodRepository.
type mockPayoutMethodRepo struct {
	SaveFunc      func(ctx context.Context, m *domain.PayoutMethod) error
	GetActiveFunc func(ctx context.Context, partnerType domain.PartnerType, partnerID string) (*domain.PayoutMethod, error)
	GetByIDFunc   func(ctx context.Context, id string) (*domain.PayoutMethod, error)
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

func (m *mockPayoutMethodRepo) GetByID(ctx context.Context, id string) (*domain.PayoutMethod, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, status.Error(codes.NotFound, "method not found")
}

// ── mockPayoutRepo ─────────────────────────────────────────────────────────────

// mockPayoutRepo is a test double for domain.PayoutRepository.  Mark* methods
// default to "updated=true" so a happy-path scheduler run records terminal
// states without per-test stubbing; tests asserting the no-op (duplicate
// webhook) branch override the relevant Func to return false.
type mockPayoutRepo struct {
	CreatePendingWithLedgerFunc func(ctx context.Context, p *domain.Payout) error
	MarkProcessingFunc          func(ctx context.Context, id, providerPayoutID string) error
	MarkSucceededFunc           func(ctx context.Context, id string, completedAt time.Time) (bool, error)
	MarkFailedWithReversalFunc  func(ctx context.Context, id, reason string, completedAt time.Time) (bool, error)
	GetByIDFunc                 func(ctx context.Context, id string) (*domain.Payout, error)
	GetByProviderPayoutIDFunc   func(ctx context.Context, providerPayoutID string) (*domain.Payout, error)
	ListFunc                    func(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.Payout, error)
	ListPendingOlderThanFunc    func(ctx context.Context, olderThan time.Duration, limit int) ([]domain.Payout, error)
}

func (m *mockPayoutRepo) CreatePendingWithLedger(ctx context.Context, p *domain.Payout) error {
	if m.CreatePendingWithLedgerFunc != nil {
		return m.CreatePendingWithLedgerFunc(ctx, p)
	}
	return nil
}

func (m *mockPayoutRepo) MarkProcessing(ctx context.Context, id, providerPayoutID string) error {
	if m.MarkProcessingFunc != nil {
		return m.MarkProcessingFunc(ctx, id, providerPayoutID)
	}
	return nil
}

func (m *mockPayoutRepo) MarkSucceeded(ctx context.Context, id string, completedAt time.Time) (bool, error) {
	if m.MarkSucceededFunc != nil {
		return m.MarkSucceededFunc(ctx, id, completedAt)
	}
	return true, nil
}

func (m *mockPayoutRepo) MarkFailedWithReversal(ctx context.Context, id, reason string, completedAt time.Time) (bool, error) {
	if m.MarkFailedWithReversalFunc != nil {
		return m.MarkFailedWithReversalFunc(ctx, id, reason, completedAt)
	}
	return true, nil
}

func (m *mockPayoutRepo) GetByID(ctx context.Context, id string) (*domain.Payout, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, status.Error(codes.NotFound, "payout not found")
}

func (m *mockPayoutRepo) GetByProviderPayoutID(ctx context.Context, providerPayoutID string) (*domain.Payout, error) {
	if m.GetByProviderPayoutIDFunc != nil {
		return m.GetByProviderPayoutIDFunc(ctx, providerPayoutID)
	}
	return nil, status.Error(codes.NotFound, "payout not found")
}

func (m *mockPayoutRepo) List(ctx context.Context, partnerType domain.PartnerType, partnerID string, limit, offset int) ([]domain.Payout, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, partnerType, partnerID, limit, offset)
	}
	return nil, nil
}

func (m *mockPayoutRepo) ListPendingOlderThan(ctx context.Context, olderThan time.Duration, limit int) ([]domain.Payout, error) {
	if m.ListPendingOlderThanFunc != nil {
		return m.ListPendingOlderThanFunc(ctx, olderThan, limit)
	}
	return nil, nil
}
