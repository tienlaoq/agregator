package usecase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// testPayoutUseCase wires a PayoutUseCase to in-memory mocks with a deterministic
// MinPayoutKopecks so threshold logic is easy to reason about in tests.
func testPayoutUseCase(
	payouts *mockPayoutRepo,
	methods *mockPayoutMethodRepo,
	ledger *mockLedgerRepo,
	prov *mockPaymentProvider,
) *PayoutUseCase {
	return NewPayoutUseCase(payouts, methods, ledger, prov, "mock", PayoutSchedulerConfig{
		MinPayoutKopecks: 10000,
		TickInterval:     time.Hour,
	}, zerolog.Nop())
}

func cardMethod(partnerType domain.PartnerType, partnerID string) *domain.PayoutMethod {
	return &domain.PayoutMethod{
		ID:            "m1",
		PartnerType:   partnerType,
		PartnerID:     partnerID,
		Kind:          domain.PayoutMethodCard,
		ProviderName:  "mock",
		ProviderToken: "tok_live",
		CardLast4:     "4242",
	}
}

// ── SetPayoutMethod validation ───────────────────────────────────────────────

func TestSetPayoutMethod_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   SetPayoutMethodInput
		want codes.Code
	}{
		{
			name: "invalid partner type",
			in:   SetPayoutMethodInput{PartnerType: "stranger", PartnerID: "p1", Kind: domain.PayoutMethodCard},
			want: codes.InvalidArgument,
		},
		{
			name: "empty partner id",
			in:   SetPayoutMethodInput{PartnerType: domain.PartnerVenue, PartnerID: "  ", Kind: domain.PayoutMethodCard},
			want: codes.InvalidArgument,
		},
		{
			name: "invalid kind",
			in:   SetPayoutMethodInput{PartnerType: domain.PartnerVenue, PartnerID: "p1", Kind: "crypto"},
			want: codes.InvalidArgument,
		},
		{
			name: "card provider mismatch",
			in: SetPayoutMethodInput{
				PartnerType: domain.PartnerVenue, PartnerID: "p1",
				Kind: domain.PayoutMethodCard, ProviderName: "stripe",
			},
			want: codes.InvalidArgument,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			uc := testPayoutUseCase(&mockPayoutRepo{}, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, &mockPaymentProvider{})
			_, err := uc.SetPayoutMethod(context.Background(), tt.in)
			require.Error(t, err)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

// A card method that omits the provider name inherits the active provider rather
// than being rejected.
func TestSetPayoutMethod_CardDefaultsProviderName(t *testing.T) {
	t.Parallel()

	var saved *domain.PayoutMethod
	methods := &mockPayoutMethodRepo{SaveFunc: func(_ context.Context, m *domain.PayoutMethod) error {
		m.ID = "m1"
		saved = m
		return nil
	}}
	uc := testPayoutUseCase(&mockPayoutRepo{}, methods, &mockLedgerRepo{}, &mockPaymentProvider{})

	got, err := uc.SetPayoutMethod(context.Background(), SetPayoutMethodInput{
		PartnerType: domain.PartnerMaster, PartnerID: "p1",
		Kind: domain.PayoutMethodCard, CardLast4: "4242", ProviderToken: "tok",
	})
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "mock", saved.ProviderName, "active provider should be filled in")
	assert.Equal(t, "m1", got.ID)
}

func TestSetPayoutMethod_BankAccount_TrimsAndSaves(t *testing.T) {
	t.Parallel()

	var saved *domain.PayoutMethod
	methods := &mockPayoutMethodRepo{SaveFunc: func(_ context.Context, m *domain.PayoutMethod) error {
		saved = m
		return nil
	}}
	uc := testPayoutUseCase(&mockPayoutRepo{}, methods, &mockLedgerRepo{}, &mockPaymentProvider{})

	_, err := uc.SetPayoutMethod(context.Background(), SetPayoutMethodInput{
		PartnerType: domain.PartnerVenue, PartnerID: " p1 ",
		Kind:    domain.PayoutMethodBankAccount,
		BankBIC: " 044525225 ", BankAccount: " 40817810099910004312 ",
		RecipientName: " ООО Баня ", RecipientINN: " 7707083893 ",
	})
	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "p1", saved.PartnerID)
	assert.Equal(t, "044525225", saved.BankBIC)
	assert.Equal(t, "40817810099910004312", saved.BankAccount)
	assert.Equal(t, "ООО Баня", saved.RecipientName)
}

// ── Scheduler: payoutPartner state machine ────────────────────────────────────

// ripeLedger returns a ledger mock that enumerates exactly one ripe partner and
// reports an available balance of availKopecks for it.
func ripeLedger(ref domain.PartnerRef, availKopecks int64) *mockLedgerRepo {
	return &mockLedgerRepo{
		PartnersWithAvailableBalanceFunc: func(_ context.Context, _ int64, _ int) ([]domain.PartnerRef, error) {
			return []domain.PartnerRef{ref}, nil
		},
		BalanceFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PartnerBalance, error) {
			return &domain.PartnerBalance{PartnerType: pt, PartnerID: pid, AvailableKopecks: availKopecks, TotalKopecks: availKopecks}, nil
		},
	}
}

func TestScheduler_FastSuccess_MarksSucceeded(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var created *domain.Payout
	var succeededID, failedID string
	payouts := &mockPayoutRepo{
		CreatePendingWithLedgerFunc: func(_ context.Context, p *domain.Payout) error { created = p; return nil },
		MarkSucceededFunc:           func(_ context.Context, id string, _ time.Time) (bool, error) { succeededID = id; return true, nil },
		MarkFailedWithReversalFunc:  func(_ context.Context, id, _ string, _ time.Time) (bool, error) { failedID = id; return true, nil },
	}
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		return &provider.PayoutResult{ProviderPayoutID: "pp1", Status: provider.PayoutStatusSucceeded}, nil
	}}

	uc := testPayoutUseCase(payouts, methods, ripeLedger(ref, 50000), prov)
	uc.tick(context.Background())

	require.NotNil(t, created, "a pending payout must be created")
	assert.Equal(t, int64(50000), created.AmountKopecks, "payout drains the full available balance")
	assert.Equal(t, created.ID, succeededID, "the created payout must be marked succeeded")
	assert.Empty(t, failedID, "must not mark failed on a successful payout")
}

func TestScheduler_FastFailure_MarksFailedWithReversal(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerMaster, PartnerID: "m1"}

	var failedReason string
	var succeededCalled bool
	payouts := &mockPayoutRepo{
		MarkSucceededFunc: func(_ context.Context, _ string, _ time.Time) (bool, error) { succeededCalled = true; return true, nil },
		MarkFailedWithReversalFunc: func(_ context.Context, _, reason string, _ time.Time) (bool, error) {
			failedReason = reason
			return true, nil
		},
	}
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		return &provider.PayoutResult{ProviderPayoutID: "pp1", Status: provider.PayoutStatusFailed, FailureReason: "destination refused"}, nil
	}}

	uc := testPayoutUseCase(payouts, methods, ripeLedger(ref, 20000), prov)
	uc.tick(context.Background())

	assert.Equal(t, "destination refused", failedReason, "failure reason must be propagated for the reversal")
	assert.False(t, succeededCalled, "must not mark succeeded on a failed payout")
}

// On PERMANENT provider errors (4xx, validation) the ledger debit must be
// reversed — provider definitively rejected the request, no payout was issued,
// reversal is safe.
func TestScheduler_PermanentProviderError_ReversesDebit(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var reversed bool
	payouts := &mockPayoutRepo{
		MarkFailedWithReversalFunc: func(_ context.Context, _, _ string, _ time.Time) (bool, error) { reversed = true; return true, nil },
	}
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		// Plain error — not wrapped with ErrTransient — is treated as permanent.
		return nil, errors.New("invalid recipient")
	}}

	uc := testPayoutUseCase(payouts, methods, ripeLedger(ref, 20000), prov)
	uc.tick(context.Background())

	assert.True(t, reversed, "a permanent provider error must reverse the ledger debit")
}

// CRITICAL: on TRANSIENT provider errors (network timeout, 5xx) the payout row
// MUST stay in pending — the provider may have accepted the request even
// though we got no response.  Reversing here and re-issuing on the next tick
// (with a new idempotency key) would double-pay the partner.
func TestScheduler_TransientProviderError_LeavesPending(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var reversed, processingCalled, succeededCalled bool
	payouts := &mockPayoutRepo{
		MarkFailedWithReversalFunc: func(_ context.Context, _, _ string, _ time.Time) (bool, error) {
			reversed = true
			return true, nil
		},
		MarkProcessingFunc: func(_ context.Context, _, _ string) error { processingCalled = true; return nil },
		MarkSucceededFunc:  func(_ context.Context, _ string, _ time.Time) (bool, error) { succeededCalled = true; return true, nil },
	}
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		// Wrap with ErrTransient to mark as transient.
		return nil, fmt.Errorf("mock 503: %w", provider.ErrTransient)
	}}

	uc := testPayoutUseCase(payouts, methods, ripeLedger(ref, 20000), prov)
	uc.tick(context.Background())

	assert.False(t, reversed, "TRANSIENT errors must NOT reverse — provider may have accepted the request")
	assert.False(t, processingCalled, "must not advance to processing without provider response")
	assert.False(t, succeededCalled, "must not mark succeeded without provider response")
}

// Reconciliation must re-call CreatePayout with the SAME idempotency key for a
// pending payout, so the provider's deduplication returns the original payout
// (or creates it now if the original POST never landed).
func TestReconcile_StuckPending_RecoversViaSameIdempotencyKey(t *testing.T) {
	t.Parallel()

	pending := domain.Payout{
		ID:             "po-stuck",
		PartnerType:    domain.PartnerVenue,
		PartnerID:      "v1",
		AmountKopecks:  20000,
		Currency:       "RUB",
		MethodID:       "m1",
		IdempotencyKey: "payout:po-stuck",
		Status:         domain.PayoutPending,
	}

	var calledIdempotencyKey string
	var processingID, providerPayoutID, succeededID string
	payouts := &mockPayoutRepo{
		ListPendingOlderThanFunc: func(_ context.Context, _ time.Duration, _ int) ([]domain.Payout, error) {
			return []domain.Payout{pending}, nil
		},
		MarkProcessingFunc: func(_ context.Context, id, ppid string) error {
			processingID = id
			providerPayoutID = ppid
			return nil
		},
		MarkSucceededFunc: func(_ context.Context, id string, _ time.Time) (bool, error) {
			succeededID = id
			return true, nil
		},
	}
	methods := &mockPayoutMethodRepo{GetByIDFunc: func(_ context.Context, _ string) (*domain.PayoutMethod, error) {
		return cardMethod(domain.PartnerVenue, "v1"), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, req provider.PayoutRequest) (*provider.PayoutResult, error) {
		calledIdempotencyKey = req.IdempotencyKey
		// Provider returns the existing payout (deduplicated on idempotency key).
		return &provider.PayoutResult{ProviderPayoutID: "yk-original", Status: provider.PayoutStatusSucceeded}, nil
	}}

	uc := testPayoutUseCase(payouts, methods, &mockLedgerRepo{}, prov)
	uc.reconcile(context.Background())

	assert.Equal(t, pending.IdempotencyKey, calledIdempotencyKey, "reconcile must reuse the original idempotency key")
	assert.Equal(t, "po-stuck", processingID, "stuck payout must be moved to processing")
	assert.Equal(t, "yk-original", providerPayoutID, "provider_payout_id must be recorded")
	assert.Equal(t, "po-stuck", succeededID, "fast-success must be marked succeeded")
}

// If reconcile still gets a transient error, the payout stays pending for the
// next reconciliation tick — no reversal, no double-pay risk.
func TestReconcile_StillTransient_LeavesPending(t *testing.T) {
	t.Parallel()

	pending := domain.Payout{
		ID: "po-stuck", PartnerType: domain.PartnerVenue, PartnerID: "v1",
		AmountKopecks: 20000, Currency: "RUB", MethodID: "m1",
		IdempotencyKey: "payout:po-stuck", Status: domain.PayoutPending,
	}

	var reversed bool
	payouts := &mockPayoutRepo{
		ListPendingOlderThanFunc: func(_ context.Context, _ time.Duration, _ int) ([]domain.Payout, error) {
			return []domain.Payout{pending}, nil
		},
		MarkFailedWithReversalFunc: func(_ context.Context, _, _ string, _ time.Time) (bool, error) { reversed = true; return true, nil },
	}
	methods := &mockPayoutMethodRepo{GetByIDFunc: func(_ context.Context, _ string) (*domain.PayoutMethod, error) {
		return cardMethod(domain.PartnerVenue, "v1"), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		return nil, fmt.Errorf("still 503: %w", provider.ErrTransient)
	}}

	uc := testPayoutUseCase(payouts, methods, &mockLedgerRepo{}, prov)
	uc.reconcile(context.Background())

	assert.False(t, reversed, "transient error during reconcile must NOT reverse")
}

// A permanent error during reconcile (provider definitively rejects the
// idempotent request) reverses the ledger — at this point we know no payout
// was issued.
func TestReconcile_PermanentError_Reverses(t *testing.T) {
	t.Parallel()

	pending := domain.Payout{
		ID: "po-stuck", PartnerType: domain.PartnerVenue, PartnerID: "v1",
		AmountKopecks: 20000, Currency: "RUB", MethodID: "m1",
		IdempotencyKey: "payout:po-stuck", Status: domain.PayoutPending,
	}

	var reversed bool
	payouts := &mockPayoutRepo{
		ListPendingOlderThanFunc: func(_ context.Context, _ time.Duration, _ int) ([]domain.Payout, error) {
			return []domain.Payout{pending}, nil
		},
		MarkFailedWithReversalFunc: func(_ context.Context, _, _ string, _ time.Time) (bool, error) { reversed = true; return true, nil },
	}
	methods := &mockPayoutMethodRepo{GetByIDFunc: func(_ context.Context, _ string) (*domain.PayoutMethod, error) {
		return cardMethod(domain.PartnerVenue, "v1"), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		return nil, errors.New("invalid recipient")
	}}

	uc := testPayoutUseCase(payouts, methods, &mockLedgerRepo{}, prov)
	uc.reconcile(context.Background())

	assert.True(t, reversed, "permanent error during reconcile must reverse the ledger")
}

func TestScheduler_ProviderPending_LeavesProcessing(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var markProcessingID, succeededCalled, failedCalled bool
	var procID string
	payouts := &mockPayoutRepo{
		MarkProcessingFunc:         func(_ context.Context, id, _ string) error { markProcessingID = true; procID = id; return nil },
		MarkSucceededFunc:          func(_ context.Context, _ string, _ time.Time) (bool, error) { succeededCalled = true; return true, nil },
		MarkFailedWithReversalFunc: func(_ context.Context, _, _ string, _ time.Time) (bool, error) { failedCalled = true; return true, nil },
	}
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		return &provider.PayoutResult{ProviderPayoutID: "pp1", Status: provider.PayoutStatusPending}, nil
	}}

	uc := testPayoutUseCase(payouts, methods, ripeLedger(ref, 20000), prov)
	uc.tick(context.Background())

	assert.True(t, markProcessingID, "pending payout must be marked processing")
	assert.NotEmpty(t, procID)
	assert.False(t, succeededCalled, "pending must not be marked succeeded — webhook will")
	assert.False(t, failedCalled, "pending must not be marked failed — webhook will")
}

// Re-reading the balance inside payoutPartner guards against a stale enumeration:
// if the balance dropped below the threshold (e.g. a refund landed), no payout
// is created.
func TestScheduler_BalanceDroppedBelowMin_NoPayout(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var created bool
	payouts := &mockPayoutRepo{CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error { created = true; return nil }}
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}

	// Enumeration says ripe, but the fresh Balance read is below min (9999 < 10000).
	uc := testPayoutUseCase(payouts, methods, ripeLedger(ref, 9999), &mockPaymentProvider{})
	uc.tick(context.Background())

	assert.False(t, created, "no payout when the re-read balance is below the threshold")
}

func TestScheduler_NoActiveMethod_Skips(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var created bool
	payouts := &mockPayoutRepo{CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error { created = true; return nil }}
	// methods default GetActive returns NotFound.
	uc := testPayoutUseCase(payouts, &mockPayoutMethodRepo{}, ripeLedger(ref, 50000), &mockPaymentProvider{})
	uc.tick(context.Background())

	assert.False(t, created, "a partner with balance but no payout method must be skipped, not paid")
}

// CreatePendingWithLedger returning FailedPrecondition means the live balance
// inside the repo TX was lower than the planned amount — a refund-reversal
// landed between our Balance read and the insert.  The scheduler must back off
// silently and re-evaluate on the next tick, NOT propagate the error.
func TestScheduler_BalanceRaceFailedPrecondition_NoProviderCall(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	payouts := &mockPayoutRepo{CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error {
		return status.Error(codes.FailedPrecondition, "available=10000 < requested=50000")
	}}
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}
	var providerCalled bool
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		providerCalled = true
		return &provider.PayoutResult{Status: provider.PayoutStatusSucceeded}, nil
	}}

	uc := testPayoutUseCase(payouts, methods, ripeLedger(ref, 50000), prov)
	uc.tick(context.Background())

	assert.False(t, providerCalled, "FailedPrecondition (stale balance) must short-circuit before provider call")
}

// CreatePendingWithLedger returning AlreadyExists means another tick (or an
// in-flight payout) already claimed this partner; the scheduler must back off
// without calling the provider.
func TestScheduler_AlreadyExists_NoProviderCall(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	payouts := &mockPayoutRepo{CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error {
		return status.Error(codes.AlreadyExists, "in-flight payout exists")
	}}
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}
	var providerCalled bool
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		providerCalled = true
		return &provider.PayoutResult{Status: provider.PayoutStatusSucceeded}, nil
	}}

	uc := testPayoutUseCase(payouts, methods, ripeLedger(ref, 50000), prov)
	uc.tick(context.Background())

	assert.False(t, providerCalled, "must not call the provider when the payout row could not be claimed")
}

// ── HandlePayoutWebhook ──────────────────────────────────────────────────────

func TestHandlePayoutWebhook_Succeeded(t *testing.T) {
	t.Parallel()

	var succeededID string
	payouts := &mockPayoutRepo{
		GetByProviderPayoutIDFunc: func(_ context.Context, _ string) (*domain.Payout, error) {
			return &domain.Payout{ID: "po1", ProviderName: "mock", Status: domain.PayoutProcessing}, nil
		},
		MarkSucceededFunc: func(_ context.Context, id string, _ time.Time) (bool, error) { succeededID = id; return true, nil },
	}
	prov := &mockPaymentProvider{ParsePayoutWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*provider.PayoutWebhookEvent, error) {
		return &provider.PayoutWebhookEvent{ProviderPayoutID: "pp1", Status: provider.PayoutStatusSucceeded}, nil
	}}

	uc := testPayoutUseCase(payouts, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, prov)
	ev, err := uc.HandlePayoutWebhook(context.Background(), []byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, ev)
	assert.Equal(t, "po1", succeededID)
}

// Provider-mismatch guard: a webhook from one provider must not mutate a payout
// row created by another provider — protects T-Bank/Sber rows from stale
// notifications arriving after a provider migration.
func TestHandlePayoutWebhook_ProviderMismatch_Rejected(t *testing.T) {
	t.Parallel()

	var succeededCalled, failedCalled bool
	payouts := &mockPayoutRepo{
		GetByProviderPayoutIDFunc: func(_ context.Context, _ string) (*domain.Payout, error) {
			// Row was created by mock (e.g. before a T-Bank migration).
			return &domain.Payout{ID: "po1", ProviderName: "mock", Status: domain.PayoutProcessing}, nil
		},
		MarkSucceededFunc:          func(_ context.Context, _ string, _ time.Time) (bool, error) { succeededCalled = true; return true, nil },
		MarkFailedWithReversalFunc: func(_ context.Context, _, _ string, _ time.Time) (bool, error) { failedCalled = true; return true, nil },
	}
	prov := &mockPaymentProvider{ParsePayoutWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*provider.PayoutWebhookEvent, error) {
		return &provider.PayoutWebhookEvent{ProviderPayoutID: "pp1", Status: provider.PayoutStatusSucceeded}, nil
	}}

	// Active provider is now t-bank; the row says mock.
	uc := NewPayoutUseCase(payouts, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, prov, "tbank", PayoutSchedulerConfig{}, zerolog.Nop())

	_, err := uc.HandlePayoutWebhook(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.False(t, succeededCalled, "mismatched-provider webhook must not advance terminal state")
	assert.False(t, failedCalled, "mismatched-provider webhook must not reverse ledger")
}

// In mock/dev mode (activeProvider==""), the mismatch guard is bypassed so
// tests and local-dev flows stay simple.
func TestHandlePayoutWebhook_EmptyActiveProvider_BypassesGuard(t *testing.T) {
	t.Parallel()

	var succeededID string
	payouts := &mockPayoutRepo{
		GetByProviderPayoutIDFunc: func(_ context.Context, _ string) (*domain.Payout, error) {
			return &domain.Payout{ID: "po1", ProviderName: "mock", Status: domain.PayoutProcessing}, nil
		},
		MarkSucceededFunc: func(_ context.Context, id string, _ time.Time) (bool, error) { succeededID = id; return true, nil },
	}
	prov := &mockPaymentProvider{ParsePayoutWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*provider.PayoutWebhookEvent, error) {
		return &provider.PayoutWebhookEvent{ProviderPayoutID: "pp1", Status: provider.PayoutStatusSucceeded}, nil
	}}

	// activeProvider="" — guard skipped.
	uc := NewPayoutUseCase(payouts, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, prov, "", PayoutSchedulerConfig{}, zerolog.Nop())

	_, err := uc.HandlePayoutWebhook(context.Background(), []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "po1", succeededID)
}

func TestHandlePayoutWebhook_Failed_Reverses(t *testing.T) {
	t.Parallel()

	var failedReason string
	payouts := &mockPayoutRepo{
		GetByProviderPayoutIDFunc: func(_ context.Context, _ string) (*domain.Payout, error) {
			return &domain.Payout{ID: "po1", ProviderName: "mock", Status: domain.PayoutProcessing}, nil
		},
		MarkFailedWithReversalFunc: func(_ context.Context, _, reason string, _ time.Time) (bool, error) {
			failedReason = reason
			return true, nil
		},
	}
	prov := &mockPaymentProvider{ParsePayoutWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*provider.PayoutWebhookEvent, error) {
		return &provider.PayoutWebhookEvent{ProviderPayoutID: "pp1", Status: provider.PayoutStatusFailed, FailureReason: "bank rejected"}, nil
	}}

	uc := testPayoutUseCase(payouts, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, prov)
	_, err := uc.HandlePayoutWebhook(context.Background(), []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "bank rejected", failedReason)
}

// A webhook for a payout we don't recognise (cross-environment replay) must be
// absorbed without error and without touching any payout row.
func TestHandlePayoutWebhook_UnknownPayout_Absorbed(t *testing.T) {
	t.Parallel()

	var marked bool
	payouts := &mockPayoutRepo{
		GetByProviderPayoutIDFunc: func(_ context.Context, _ string) (*domain.Payout, error) {
			return nil, status.Error(codes.NotFound, "x")
		},
		MarkSucceededFunc:          func(_ context.Context, _ string, _ time.Time) (bool, error) { marked = true; return true, nil },
		MarkFailedWithReversalFunc: func(_ context.Context, _, _ string, _ time.Time) (bool, error) { marked = true; return true, nil },
	}
	prov := &mockPaymentProvider{ParsePayoutWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*provider.PayoutWebhookEvent, error) {
		return &provider.PayoutWebhookEvent{ProviderPayoutID: "ghost", Status: provider.PayoutStatusSucceeded}, nil
	}}

	uc := testPayoutUseCase(payouts, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, prov)
	ev, err := uc.HandlePayoutWebhook(context.Background(), []byte(`{}`))
	require.NoError(t, err)
	require.NotNil(t, ev, "the parsed event is still returned so the caller does not fall through to the payment path")
	assert.False(t, marked, "no payout row should be transitioned for an unknown provider id")
}

// A body that is not a payout notification returns (nil, nil) so the caller
// falls back to the payment webhook path.
func TestHandlePayoutWebhook_NotAPayout_ReturnsNil(t *testing.T) {
	t.Parallel()

	prov := &mockPaymentProvider{ParsePayoutWebhookFunc: func(_ context.Context, _ []byte, _ http.Header) (*provider.PayoutWebhookEvent, error) {
		return nil, nil
	}}
	uc := testPayoutUseCase(&mockPayoutRepo{}, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, prov)
	ev, err := uc.HandlePayoutWebhook(context.Background(), []byte(`{}`))
	require.NoError(t, err)
	assert.Nil(t, ev, "non-payout body must signal fall-through with a nil event")
}

// ── Destination-kind mapping ──────────────────────────────────────────────────

// toProviderDestKind decides where money physically lands, so every supported
// method kind must map to its provider destination and an unknown kind must map
// to the empty string (which the provider rejects) rather than silently routing
// to a default rail.
func TestToProviderDestKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind domain.PayoutMethodKind
		want provider.PayoutDestinationKind
	}{
		{domain.PayoutMethodCard, provider.PayoutDestCard},
		{domain.PayoutMethodBankAccount, provider.PayoutDestBankAccount},
		{domain.PayoutMethodSBP, provider.PayoutDestSBP},
		{domain.PayoutMethodKind("unknown"), ""},
		{domain.PayoutMethodKind(""), ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, toProviderDestKind(tt.kind))
		})
	}
}

// ── GetBalance guard ──────────────────────────────────────────────────────────

// An invalid partner_type must be rejected before the ledger is queried, so a
// caller cannot probe balances under a bogus partner type.
func TestGetBalance_InvalidPartnerType_Rejected(t *testing.T) {
	t.Parallel()

	var queried bool
	ledger := &mockLedgerRepo{BalanceFunc: func(_ context.Context, _ domain.PartnerType, _ string) (*domain.PartnerBalance, error) {
		queried = true
		return &domain.PartnerBalance{}, nil
	}}
	uc := testPayoutUseCase(&mockPayoutRepo{}, &mockPayoutMethodRepo{}, ledger, &mockPaymentProvider{})

	_, err := uc.GetBalance(context.Background(), "stranger", "p1")
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.False(t, queried, "ledger must not be queried for an invalid partner type")
}

// A valid partner type passes the guard and returns the ledger's balance.
func TestGetBalance_ValidPartnerType_ReturnsLedgerBalance(t *testing.T) {
	t.Parallel()

	ledger := &mockLedgerRepo{BalanceFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PartnerBalance, error) {
		assert.Equal(t, domain.PartnerVenue, pt)
		assert.Equal(t, "v1", pid)
		return &domain.PartnerBalance{TotalKopecks: 5000, AvailableKopecks: 3000}, nil
	}}
	uc := testPayoutUseCase(&mockPayoutRepo{}, &mockPayoutMethodRepo{}, ledger, &mockPaymentProvider{})

	bal, err := uc.GetBalance(context.Background(), domain.PartnerVenue, "v1")
	require.NoError(t, err)
	require.NotNil(t, bal)
	assert.Equal(t, int64(5000), bal.TotalKopecks)
	assert.Equal(t, int64(3000), bal.AvailableKopecks)
}
