package usecase

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

func TestStatusToSubject(t *testing.T) {
	tests := []struct {
		status domain.PaymentStatus
		want   domain.OutboxSubject
	}{
		{domain.StatusSucceeded, domain.SubjectPaymentCompleted},
		{domain.StatusCancelled, domain.SubjectPaymentFailed},
		{domain.StatusRefunded, domain.SubjectPaymentRefunded},
		{domain.StatusPending, domain.SubjectPaymentFailed}, // default arm
		{domain.PaymentStatus("bogus"), domain.SubjectPaymentFailed},
	}
	for _, tt := range tests {
		if got := statusToSubject(tt.status); got != tt.want {
			t.Errorf("statusToSubject(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestBuildOutboxEvent(t *testing.T) {
	p := &domain.Payment{ID: "p1", BookingID: "b1", Status: domain.StatusSucceeded}
	ev, err := buildOutboxEvent(p, domain.SubjectPaymentCompleted)
	require.NoError(t, err)
	require.Equal(t, domain.SubjectPaymentCompleted, ev.Subject)
	require.Equal(t, "p1", ev.PaymentID)

	var payload struct {
		PaymentID string `json:"payment_id"`
		BookingID string `json:"booking_id"`
		Status    string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	require.Equal(t, "p1", payload.PaymentID)
	require.Equal(t, "b1", payload.BookingID)
	require.Equal(t, "succeeded", payload.Status)
}

func TestPlatformFee_EdgeCases(t *testing.T) {
	// Non-positive gross or fee → zero (guard arms).
	assert.Equal(t, int64(0), PlatformFeeKopecks(0, 1500))
	assert.Equal(t, int64(0), PlatformFeeKopecks(-100, 1500))
	assert.Equal(t, int64(0), PlatformFeeKopecks(10000, 0))
	assert.Equal(t, int64(0), PlatformFeeKopecks(10000, -1))

	// Fee larger than gross → net clamped to zero (never negative).
	assert.Equal(t, int64(0), CounterpartyNetKopecks(1000, 2000))
}

func TestPayoutReads_PassThrough(t *testing.T) {
	ctx := context.Background()

	t.Run("GetActivePayoutMethod", func(t *testing.T) {
		methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
			return &domain.PayoutMethod{ID: "pm1", PartnerType: pt, PartnerID: pid}, nil
		}}
		uc := testPayoutUseCase(&mockPayoutRepo{}, methods, &mockLedgerRepo{}, &mockPaymentProvider{})
		m, err := uc.GetActivePayoutMethod(ctx, domain.PartnerMaster, "m1")
		require.NoError(t, err)
		require.Equal(t, "pm1", m.ID)
	})

	t.Run("ListLedger", func(t *testing.T) {
		ledger := &mockLedgerRepo{ListFunc: func(context.Context, domain.PartnerType, string, int, int) ([]domain.LedgerEntry, error) {
			return []domain.LedgerEntry{{ID: 1}, {ID: 2}}, nil
		}}
		uc := testPayoutUseCase(&mockPayoutRepo{}, &mockPayoutMethodRepo{}, ledger, &mockPaymentProvider{})
		entries, err := uc.ListLedger(ctx, domain.PartnerMaster, "m1", 10, 0)
		require.NoError(t, err)
		require.Len(t, entries, 2)
	})

	t.Run("ListPayouts", func(t *testing.T) {
		payouts := &mockPayoutRepo{ListFunc: func(context.Context, domain.PartnerType, string, int, int) ([]domain.Payout, error) {
			return []domain.Payout{{ID: "po1"}}, nil
		}}
		uc := testPayoutUseCase(payouts, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, &mockPaymentProvider{})
		ps, err := uc.ListPayouts(ctx, domain.PartnerMaster, "m1", 10, 0)
		require.NoError(t, err)
		require.Len(t, ps, 1)
	})
}

func TestStartSchedulerInBackground_StartsAndStops(t *testing.T) {
	// Default mocks: PartnersWithAvailableBalance and ListPendingOlderThan return
	// nil, so the immediate tick + reconcile are quick no-ops. The returned stop
	// func must cancel the loop and wait for the goroutine to exit (no leak).
	uc := testPayoutUseCase(&mockPayoutRepo{}, &mockPayoutMethodRepo{}, &mockLedgerRepo{}, &mockPaymentProvider{})

	stop := uc.StartSchedulerInBackground(context.Background())

	done := make(chan struct{})
	go func() { stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop() did not return — scheduler goroutine may be leaking")
	}
}
