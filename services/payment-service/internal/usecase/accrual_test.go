package usecase

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

const testHold = 24 * time.Hour

// paymentUCWithLedger builds a PaymentUseCase with a caller-supplied ledger mock
// and a known hold duration, so accrual/reversal side-effects can be asserted.
// activeProvider="" disables the provider-mismatch guard.
func paymentUCWithLedger(repo *mockPaymentRepo, ledger *mockLedgerRepo, prov *mockPaymentProvider) *PaymentUseCase {
	return NewPaymentUseCase(repo, &mockOutboxRepo{}, ledger, prov, "", "https://example.com/return", 1500, testHold)
}

// ── Accrual on payment success ────────────────────────────────────────────────

// A successful webhook must credit the partner's ledger with the frozen net
// amount, and the accrual must become available at ServiceAt + hold — the
// usecase-side half of the escrow hold mechanic.
func TestHandleWebhook_Success_WritesAccrual(t *testing.T) {
	t.Parallel()

	serviceAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	providerID := "pay_123"
	p := &domain.Payment{
		ID: "pay-1", BookingID: "book-1", Status: domain.StatusPending, ProviderID: providerID,
		CounterpartyType: "venue", CounterpartyID: "v1", CounterpartyNetKopecks: 4250,
		ServiceAt: serviceAt,
	}

	repo := &mockPaymentRepo{
		GetByProviderIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) { return p, nil },
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			return true, nil
		},
	}
	var accrual *domain.LedgerEntry
	ledger := &mockLedgerRepo{AppendAccrualFunc: func(_ context.Context, e *domain.LedgerEntry) error {
		accrual = e
		return nil
	}}
	prov := &mockPaymentProvider{ParseWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*domain.WebhookEvent, error) {
		return &domain.WebhookEvent{ProviderPaymentID: providerID, Status: domain.StatusSucceeded}, nil
	}}

	uc := paymentUCWithLedger(repo, ledger, prov)
	_, err := uc.HandleWebhook(context.Background(), []byte(`{}`))
	require.NoError(t, err)

	require.NotNil(t, accrual, "a ledger accrual must be written on success")
	assert.Equal(t, domain.PartnerVenue, accrual.PartnerType)
	assert.Equal(t, "v1", accrual.PartnerID)
	assert.Equal(t, domain.LedgerAccrual, accrual.EntryType)
	assert.Equal(t, int64(4250), accrual.AmountKopecks, "accrual is the frozen counterparty net")
	assert.Equal(t, "pay-1", accrual.PaymentID)
	assert.Equal(t, serviceAt.Add(testHold), accrual.AvailableAt, "hold = ServiceAt + holdDuration")
}

// When the payment carries no ServiceAt, the hold is measured from now.
func TestWriteAccrual_NoServiceAt_HoldFromNow(t *testing.T) {
	t.Parallel()

	var accrual *domain.LedgerEntry
	ledger := &mockLedgerRepo{AppendAccrualFunc: func(_ context.Context, e *domain.LedgerEntry) error {
		accrual = e
		return nil
	}}
	uc := paymentUCWithLedger(&mockPaymentRepo{}, ledger, &mockPaymentProvider{})

	before := time.Now()
	err := uc.writeAccrual(context.Background(), &domain.Payment{
		ID: "pay-1", CounterpartyType: "master", CounterpartyID: "m1", CounterpartyNetKopecks: 999,
	})
	require.NoError(t, err)
	require.NotNil(t, accrual)

	lo, hi := before.Add(testHold), time.Now().Add(testHold)
	assert.Falsef(t, accrual.AvailableAt.Before(lo), "available_at %v earlier than now+hold lower bound %v", accrual.AvailableAt, lo)
	assert.Falsef(t, accrual.AvailableAt.After(hi), "available_at %v later than now+hold upper bound %v", accrual.AvailableAt, hi)
}

func TestWriteAccrual_NoCounterparty_NoOp(t *testing.T) {
	t.Parallel()

	var called bool
	ledger := &mockLedgerRepo{AppendAccrualFunc: func(_ context.Context, _ *domain.LedgerEntry) error { called = true; return nil }}
	uc := paymentUCWithLedger(&mockPaymentRepo{}, ledger, &mockPaymentProvider{})

	// No counterparty → nothing to credit.
	require.NoError(t, uc.writeAccrual(context.Background(), &domain.Payment{ID: "p", CounterpartyNetKopecks: 100}))
	assert.False(t, called, "no accrual when the payment has no counterparty")
}

func TestWriteAccrual_NonPositiveNet_NoOp(t *testing.T) {
	t.Parallel()

	var called bool
	ledger := &mockLedgerRepo{AppendAccrualFunc: func(_ context.Context, _ *domain.LedgerEntry) error { called = true; return nil }}
	uc := paymentUCWithLedger(&mockPaymentRepo{}, ledger, &mockPaymentProvider{})

	require.NoError(t, uc.writeAccrual(context.Background(), &domain.Payment{
		ID: "p", CounterpartyType: "venue", CounterpartyID: "v1", CounterpartyNetKopecks: 0,
	}))
	assert.False(t, called, "no accrual when the net share is zero")
}

// A retried successful webhook collapses to one accrual: the repo signals the
// duplicate via AlreadyExists and the usecase absorbs it.
func TestWriteAccrual_AlreadyExists_Absorbed(t *testing.T) {
	t.Parallel()

	ledger := &mockLedgerRepo{AppendAccrualFunc: func(_ context.Context, _ *domain.LedgerEntry) error {
		return status.Error(codes.AlreadyExists, "accrual exists")
	}}
	uc := paymentUCWithLedger(&mockPaymentRepo{}, ledger, &mockPaymentProvider{})

	err := uc.writeAccrual(context.Background(), &domain.Payment{
		ID: "p", CounterpartyType: "venue", CounterpartyID: "v1", CounterpartyNetKopecks: 100,
	})
	assert.NoError(t, err, "duplicate accrual must be absorbed, not surfaced as an error")
}

// ── Reversal on refund ───────────────────────────────────────────────────────

// Refunding a succeeded booking payment must reverse the partner's accrual with
// a negative entry that points back at the original accrual.
func TestRefundByBooking_WritesReversal(t *testing.T) {
	t.Parallel()

	p := &domain.Payment{
		ID: "pay-1", BookingID: "book-1", Status: domain.StatusSucceeded, ProviderID: "pp1",
		CounterpartyType: "venue", CounterpartyID: "v1",
	}
	repo := &mockPaymentRepo{
		GetByBookingIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) { return p, nil },
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, st domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			assert.Equal(t, domain.StatusRefunded, st)
			return true, nil
		},
	}
	var reversal *domain.LedgerEntry
	ledger := &mockLedgerRepo{
		FindAccrualByPaymentFunc: func(_ context.Context, _ string) (*domain.LedgerEntry, error) {
			return &domain.LedgerEntry{ID: 7, PartnerType: domain.PartnerVenue, PartnerID: "v1", AmountKopecks: 4250}, nil
		},
		AppendReversalFunc: func(_ context.Context, e *domain.LedgerEntry) error { reversal = e; return nil },
	}
	prov := &mockPaymentProvider{} // Refund defaults to success

	uc := paymentUCWithLedger(repo, ledger, prov)
	require.NoError(t, uc.RefundByBooking(context.Background(), "book-1"))

	require.NotNil(t, reversal, "a reversal must be written on refund")
	assert.Equal(t, domain.LedgerReversal, reversal.EntryType)
	assert.Equal(t, int64(-4250), reversal.AmountKopecks, "reversal negates the original accrual")
	assert.Equal(t, int64(7), reversal.ReversesEntryID, "reversal references the accrual it undoes")
	assert.Equal(t, "pay-1", reversal.PaymentID)
	assert.Equal(t, "refund", reversal.Reason)
}

// A refund with no prior accrual (defensive — shouldn't happen for a succeeded
// payment) must be a no-op, not an error.
func TestWriteReversal_NoAccrual_NoOp(t *testing.T) {
	t.Parallel()

	var called bool
	ledger := &mockLedgerRepo{
		FindAccrualByPaymentFunc: func(_ context.Context, _ string) (*domain.LedgerEntry, error) {
			return nil, status.Error(codes.NotFound, "no accrual")
		},
		AppendReversalFunc: func(_ context.Context, _ *domain.LedgerEntry) error { called = true; return nil },
	}
	uc := paymentUCWithLedger(&mockPaymentRepo{}, ledger, &mockPaymentProvider{})

	err := uc.writeReversal(context.Background(), &domain.Payment{ID: "p"})
	require.NoError(t, err)
	assert.False(t, called, "no reversal when there is no accrual to undo")
}

// A refund for a booking with no payment record is a silent no-op: the partner
// ledger must not be touched and no error surfaces to the caller (the booking
// event is acknowledged, not retried forever).
func TestRefundByBooking_UnknownBooking_NoOp(t *testing.T) {
	t.Parallel()

	var refundCalled, reversalCalled bool
	repo := &mockPaymentRepo{GetByBookingIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) {
		return nil, status.Error(codes.NotFound, "no payment for booking")
	}}
	ledger := &mockLedgerRepo{AppendReversalFunc: func(_ context.Context, _ *domain.LedgerEntry) error { reversalCalled = true; return nil }}
	prov := &mockPaymentProvider{RefundFunc: func(_ context.Context, _ string, _ int64, _ string) error { refundCalled = true; return nil }}

	uc := paymentUCWithLedger(repo, ledger, prov)
	require.NoError(t, uc.RefundByBooking(context.Background(), "ghost-booking"))
	assert.False(t, refundCalled, "no provider refund for an unknown booking")
	assert.False(t, reversalCalled, "no ledger reversal for an unknown booking")
}

// If the provider refund call fails, the payment must NOT be marked refunded and
// the accrual must NOT be reversed — otherwise we would credit the client's
// reversal in the ledger while their money was never actually returned.  The
// error propagates so the booking event is retried.
func TestRefundByBooking_ProviderRefundFails_NoStatusChange_NoReversal(t *testing.T) {
	t.Parallel()

	p := &domain.Payment{
		ID: "pay-1", BookingID: "book-1", Status: domain.StatusSucceeded, ProviderID: "pp1",
		CounterpartyType: "venue", CounterpartyID: "v1",
	}
	var statusUpdated, reversalCalled bool
	repo := &mockPaymentRepo{
		GetByBookingIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) { return p, nil },
		UpdateStatusWithOutboxFunc: func(_ context.Context, _ string, _ domain.PaymentStatus, _ string, _ *domain.OutboxEvent) (bool, error) {
			statusUpdated = true
			return true, nil
		},
	}
	ledger := &mockLedgerRepo{AppendReversalFunc: func(_ context.Context, _ *domain.LedgerEntry) error { reversalCalled = true; return nil }}
	prov := &mockPaymentProvider{RefundFunc: func(_ context.Context, _ string, _ int64, _ string) error {
		return status.Error(codes.Unavailable, "provider down")
	}}

	uc := paymentUCWithLedger(repo, ledger, prov)
	err := uc.RefundByBooking(context.Background(), "book-1")
	require.Error(t, err, "a failed provider refund must surface as an error for retry")
	assert.False(t, statusUpdated, "payment must not be marked refunded when the provider refund failed")
	assert.False(t, reversalCalled, "no ledger reversal when the provider refund failed")
}

// A retried refund collapses to one reversal: the repo signals the duplicate via
// AlreadyExists and the usecase absorbs it.
func TestWriteReversal_AlreadyExists_Absorbed(t *testing.T) {
	t.Parallel()

	ledger := &mockLedgerRepo{
		FindAccrualByPaymentFunc: func(_ context.Context, _ string) (*domain.LedgerEntry, error) {
			return &domain.LedgerEntry{ID: 7, PartnerType: domain.PartnerVenue, PartnerID: "v1", AmountKopecks: 4250}, nil
		},
		AppendReversalFunc: func(_ context.Context, _ *domain.LedgerEntry) error {
			return status.Error(codes.AlreadyExists, "reversal exists")
		},
	}
	uc := paymentUCWithLedger(&mockPaymentRepo{}, ledger, &mockPaymentProvider{})

	err := uc.writeReversal(context.Background(), &domain.Payment{ID: "pay-1"})
	assert.NoError(t, err, "duplicate reversal must be absorbed, not surfaced as an error")
}

// A non-succeeded payment is not refunded at the provider and writes no reversal.
func TestRefundByBooking_NotSucceeded_NoOp(t *testing.T) {
	t.Parallel()

	p := &domain.Payment{ID: "pay-1", BookingID: "book-1", Status: domain.StatusPending}
	var refundCalled, reversalCalled bool
	repo := &mockPaymentRepo{GetByBookingIDFunc: func(_ context.Context, _ string) (*domain.Payment, error) { return p, nil }}
	ledger := &mockLedgerRepo{AppendReversalFunc: func(_ context.Context, _ *domain.LedgerEntry) error { reversalCalled = true; return nil }}
	prov := &mockPaymentProvider{RefundFunc: func(_ context.Context, _ string, _ int64, _ string) error { refundCalled = true; return nil }}

	uc := paymentUCWithLedger(repo, ledger, prov)
	require.NoError(t, uc.RefundByBooking(context.Background(), "book-1"))
	assert.False(t, refundCalled, "pending payment must not be refunded")
	assert.False(t, reversalCalled, "pending payment must not produce a reversal")
}

// ── Payout status state machine ───────────────────────────────────────────────

func TestPayoutStatus_IsTerminal(t *testing.T) {
	t.Parallel()
	assert.True(t, domain.PayoutSucceeded.IsTerminal())
	assert.True(t, domain.PayoutFailed.IsTerminal())
	assert.False(t, domain.PayoutPending.IsTerminal())
	assert.False(t, domain.PayoutProcessing.IsTerminal())
}

// Ensure the test stays honest if provider statuses are renamed.
var _ = provider.PayoutStatusSucceeded
