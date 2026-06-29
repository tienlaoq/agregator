//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

func newTestPayout(partner, methodID string) *domain.Payout {
	return &domain.Payout{
		PartnerType:        domain.PartnerVenue,
		PartnerID:          partner,
		AmountKopecks:      20000,
		Currency:           "RUB",
		MethodID:           methodID,
		MethodKindSnapshot: domain.PayoutMethodCard,
		MethodDisplay:      "•••• 4242",
		ProviderName:       "mock",
		IdempotencyKey:     "idem_" + uuid.NewString(),
	}
}

// CreatePendingWithLedger must persist the payout row and a matching negative
// ledger debit in one transaction, so the debit nets against the balance the
// instant the payout exists.
func TestIntegration_Payout_CreatePendingWithLedgerDebit(t *testing.T) {
	ctx := context.Background()
	ledger := NewLedgerRepo(testPool)
	payouts := NewPayoutRepo(testPool)
	partner := newPartnerID()
	methodID := seedCardMethod(ctx, t, domain.PartnerVenue, partner)

	seedAccrual(ctx, t, ledger, domain.PartnerVenue, partner, 50000, time.Now().Add(-time.Hour))

	p := newTestPayout(partner, methodID)
	require.NoError(t, payouts.CreatePendingWithLedger(ctx, p))
	assert.NotEmpty(t, p.ID)

	got, err := payouts.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PayoutPending, got.Status)
	assert.Equal(t, int64(20000), got.AmountKopecks)

	bal, err := ledger.Balance(ctx, domain.PartnerVenue, partner)
	require.NoError(t, err)
	assert.Equal(t, int64(30000), bal.TotalKopecks, "ledger debit must net against accrual")
	assert.Equal(t, int64(30000), bal.AvailableKopecks)
}

// The real double-spend guard: two concurrent payouts for the SAME partner
// with DIFFERENT idempotency keys. Only the payouts_partner_active_uniq partial
// index can stop the second — it must fail at commit with 23505, translated to
// AlreadyExists, leaving exactly one payout row.
func TestIntegration_Payout_DoubleSpendBlockedByPartnerActiveIndex(t *testing.T) {
	ctx := context.Background()
	ledger := NewLedgerRepo(testPool)
	payouts := NewPayoutRepo(testPool)
	partner := newPartnerID()
	methodID := seedCardMethod(ctx, t, domain.PartnerVenue, partner)

	seedAccrual(ctx, t, ledger, domain.PartnerVenue, partner, 100000, time.Now().Add(-time.Hour))

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			p := newTestPayout(partner, methodID) // distinct idempotency key per attempt
			<-start
			errs[idx] = payouts.CreatePendingWithLedger(ctx, p)
		}(i)
	}
	close(start)
	wg.Wait()

	var won, conflicted int
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case statusCode(err) == codes.AlreadyExists:
			conflicted++
		default:
			t.Fatalf("unexpected error from CreatePendingWithLedger: %v", err)
		}
	}
	assert.Equal(t, 1, won, "exactly one concurrent payout may win")
	assert.Equal(t, 1, conflicted, "the loser must be rejected with AlreadyExists")

	list, err := payouts.List(ctx, domain.PartnerVenue, partner, 100, 0)
	require.NoError(t, err)
	assert.Len(t, list, 1, "only one payout row may persist for the partner")
}

// payouts_idempotency_key_uniq is the second guard: a retried scheduler tick
// reusing a key must collapse to one payout even across partners. Using a
// different partner isolates the collision to the idempotency index (the
// partner_active index cannot fire for a partner with no open payout).
func TestIntegration_Payout_IdempotencyKeyUnique(t *testing.T) {
	ctx := context.Background()
	payouts := NewPayoutRepo(testPool)

	partnerA := newPartnerID()
	partnerB := newPartnerID()
	methodA := seedCardMethod(ctx, t, domain.PartnerVenue, partnerA)
	methodB := seedCardMethod(ctx, t, domain.PartnerVenue, partnerB)

	key := "idem_shared_" + uuid.NewString()

	first := newTestPayout(partnerA, methodA)
	first.IdempotencyKey = key
	require.NoError(t, payouts.CreatePendingWithLedger(ctx, first))

	second := newTestPayout(partnerB, methodB)
	second.IdempotencyKey = key
	err := payouts.CreatePendingWithLedger(ctx, second)
	require.Error(t, err)
	assert.Equal(t, codes.AlreadyExists, statusCode(err))

	list, err := payouts.List(ctx, domain.PartnerVenue, partnerB, 100, 0)
	require.NoError(t, err)
	assert.Empty(t, list, "the rolled-back payout must not persist")
}

// MarkFailedWithReversal must atomically flip the payout to failed AND append a
// compensating ledger entry that restores the partner's balance — and be a
// no-op on a second call (no double reversal).
func TestIntegration_Payout_MarkFailedWithReversal(t *testing.T) {
	ctx := context.Background()
	ledger := NewLedgerRepo(testPool)
	payouts := NewPayoutRepo(testPool)
	partner := newPartnerID()
	methodID := seedCardMethod(ctx, t, domain.PartnerVenue, partner)

	seedAccrual(ctx, t, ledger, domain.PartnerVenue, partner, 30000, time.Now().Add(-time.Hour))

	p := newTestPayout(partner, methodID)
	p.AmountKopecks = 12000
	require.NoError(t, payouts.CreatePendingWithLedger(ctx, p))

	afterDebit, err := ledger.Balance(ctx, domain.PartnerVenue, partner)
	require.NoError(t, err)
	assert.Equal(t, int64(18000), afterDebit.AvailableKopecks, "debit must reduce available balance")

	updated, err := payouts.MarkFailedWithReversal(ctx, p.ID, "provider rejected", time.Now())
	require.NoError(t, err)
	assert.True(t, updated)

	got, err := payouts.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PayoutFailed, got.Status)
	assert.Equal(t, "provider rejected", got.FailureReason)
	assert.False(t, got.CompletedAt.IsZero())

	restored, err := ledger.Balance(ctx, domain.PartnerVenue, partner)
	require.NoError(t, err)
	assert.Equal(t, int64(30000), restored.AvailableKopecks, "reversal must restore the balance")

	// Idempotent: a second failure must not write another reversal.
	updated, err = payouts.MarkFailedWithReversal(ctx, p.ID, "again", time.Now())
	require.NoError(t, err)
	assert.False(t, updated, "already-terminal payout must report no update")

	stillRestored, err := ledger.Balance(ctx, domain.PartnerVenue, partner)
	require.NoError(t, err)
	assert.Equal(t, int64(30000), stillRestored.AvailableKopecks, "no double reversal")
}

func TestIntegration_Payout_MarkFailedWithReversal_NotFound(t *testing.T) {
	ctx := context.Background()
	payouts := NewPayoutRepo(testPool)

	_, err := payouts.MarkFailedWithReversal(ctx, uuid.NewString(), "x", time.Now())
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, statusCode(err))
}
