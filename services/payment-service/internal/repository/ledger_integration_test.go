//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

// Balance must split SUM(amount) into available vs held using the available_at
// window: Total = SUM, Available excludes accruals whose available_at > now(),
// Held = Total - Available.
func TestIntegration_Ledger_Balance_HeldVsAvailable(t *testing.T) {
	ctx := context.Background()
	repo := NewLedgerRepo(testPool)
	partner := newPartnerID()
	now := time.Now()

	seedAccrual(ctx, t, repo, domain.PartnerVenue, partner, 10000, now.Add(-time.Hour)) // available
	seedAccrual(ctx, t, repo, domain.PartnerVenue, partner, 5000, now.Add(time.Hour))   // held

	bal, err := repo.Balance(ctx, domain.PartnerVenue, partner)
	require.NoError(t, err)
	assert.Equal(t, int64(15000), bal.TotalKopecks)
	assert.Equal(t, int64(10000), bal.AvailableKopecks)
	assert.Equal(t, int64(5000), bal.HeldKopecks)
	assert.False(t, bal.LastEntryAt.IsZero())
}

// An accrual that is held now must flip into the available balance once its
// available_at passes — driven purely by the SQL now() comparison.
func TestIntegration_Ledger_HoldTransition(t *testing.T) {
	ctx := context.Background()
	repo := NewLedgerRepo(testPool)
	partner := newPartnerID()

	seedAccrual(ctx, t, repo, domain.PartnerMaster, partner, 7000, time.Now().Add(2*time.Second))

	before, err := repo.Balance(ctx, domain.PartnerMaster, partner)
	require.NoError(t, err)
	assert.Equal(t, int64(7000), before.TotalKopecks)
	assert.Equal(t, int64(0), before.AvailableKopecks)
	assert.Equal(t, int64(7000), before.HeldKopecks)

	time.Sleep(2500 * time.Millisecond)

	after, err := repo.Balance(ctx, domain.PartnerMaster, partner)
	require.NoError(t, err)
	assert.Equal(t, int64(7000), after.TotalKopecks)
	assert.Equal(t, int64(7000), after.AvailableKopecks, "accrual must become available once available_at passes")
	assert.Equal(t, int64(0), after.HeldKopecks)
}

// PartnersWithAvailableBalance must return only partners whose *available*
// balance is >= min — held-only money does not count, and below-min partners
// are excluded.
func TestIntegration_Ledger_PartnersWithAvailableBalance(t *testing.T) {
	ctx := context.Background()
	repo := NewLedgerRepo(testPool)
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	high := newPartnerID()
	low := newPartnerID()
	held := newPartnerID()

	seedAccrual(ctx, t, repo, domain.PartnerVenue, high, 8000, past)   // available 8000
	seedAccrual(ctx, t, repo, domain.PartnerVenue, low, 2000, past)    // available 2000
	seedAccrual(ctx, t, repo, domain.PartnerVenue, held, 9000, future) // held 9000, available 0

	const minKopecks = 5000
	refs, err := repo.PartnersWithAvailableBalance(ctx, minKopecks, 1000)
	require.NoError(t, err)

	got := make(map[string]bool, len(refs))
	for _, r := range refs {
		got[r.PartnerID] = true
	}
	assert.True(t, got[high], "partner with available >= min must be returned")
	assert.False(t, got[low], "partner below min must be excluded")
	assert.False(t, got[held], "partner with only held balance must be excluded")
}

func TestIntegration_Ledger_PartnersWithAvailableBalance_RejectsBadMin(t *testing.T) {
	ctx := context.Background()
	repo := NewLedgerRepo(testPool)

	_, err := repo.PartnersWithAvailableBalance(ctx, 0, 100)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, statusCode(err))
}
