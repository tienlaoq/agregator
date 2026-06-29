//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
)

func TestIntegration_PayoutMethod_SaveAndGetActive(t *testing.T) {
	ctx := context.Background()
	repo := NewPayoutMethodRepo(testPool)
	partner := newPartnerID()

	m := &domain.PayoutMethod{
		PartnerType:   domain.PartnerVenue,
		PartnerID:     partner,
		Kind:          domain.PayoutMethodCard,
		ProviderName:  "mock",
		CardLast4:     "4242",
		CardBrand:     "visa",
		ProviderToken: "tok_active",
	}
	require.NoError(t, repo.Save(ctx, m))
	assert.NotEmpty(t, m.ID)

	got, err := repo.GetActive(ctx, domain.PartnerVenue, partner)
	require.NoError(t, err)
	assert.Equal(t, m.ID, got.ID)
	assert.Equal(t, domain.PayoutMethodCard, got.Kind)
	assert.Equal(t, "4242", got.CardLast4)
	assert.Equal(t, "tok_active", got.ProviderToken)
	assert.True(t, got.DeactivatedAt.IsZero())
}

// Save is an upsert: a second Save deactivates the prior active method and the
// new one becomes the single active row. The partial unique index
// partner_payout_methods_active_uniq is what keeps that invariant.
func TestIntegration_PayoutMethod_SaveReplacesActive(t *testing.T) {
	ctx := context.Background()
	repo := NewPayoutMethodRepo(testPool)
	partner := newPartnerID()

	first := &domain.PayoutMethod{
		PartnerType:   domain.PartnerMaster,
		PartnerID:     partner,
		Kind:          domain.PayoutMethodCard,
		ProviderName:  "mock",
		CardLast4:     "1111",
		ProviderToken: "tok_first",
	}
	require.NoError(t, repo.Save(ctx, first))

	second := &domain.PayoutMethod{
		PartnerType:   domain.PartnerMaster,
		PartnerID:     partner,
		Kind:          domain.PayoutMethodBankAccount,
		BankBIC:       "044525225",
		BankAccount:   "40817810099910004312",
		BankName:      "Тест Банк",
		RecipientName: "ООО Баня",
	}
	require.NoError(t, repo.Save(ctx, second))

	active, err := repo.GetActive(ctx, domain.PartnerMaster, partner)
	require.NoError(t, err)
	assert.Equal(t, second.ID, active.ID)
	assert.Equal(t, domain.PayoutMethodBankAccount, active.Kind)

	old, err := repo.GetByID(ctx, first.ID)
	require.NoError(t, err)
	assert.False(t, old.DeactivatedAt.IsZero(), "previous active method must be deactivated")
}

func TestIntegration_PayoutMethod_GetActiveNotFound(t *testing.T) {
	ctx := context.Background()
	repo := NewPayoutMethodRepo(testPool)

	_, err := repo.GetActive(ctx, domain.PartnerVenue, newPartnerID())
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, statusCode(err))
}
