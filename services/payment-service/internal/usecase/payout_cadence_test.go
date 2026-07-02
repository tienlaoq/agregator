package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/tienlao/agregator/services/payment-service/internal/domain"
	"github.com/tienlao/agregator/services/payment-service/internal/provider"
)

// weeklyUseCase wires a PayoutUseCase in weekly-cadence mode pinned to weekday,
// with a 1000 ₽ threshold matching the production default.
func weeklyUseCase(
	payouts *mockPayoutRepo,
	methods *mockPayoutMethodRepo,
	ledger *mockLedgerRepo,
	prov *mockPaymentProvider,
	weekday time.Weekday,
) *PayoutUseCase {
	return NewPayoutUseCase(payouts, methods, ledger, prov, "mock", PayoutSchedulerConfig{
		MinPayoutKopecks: 100000,
		TickInterval:     time.Hour,
		WeeklyPayout:     true,
		PayoutWeekday:    weekday,
	}, zerolog.Nop())
}

func succeedingMethodAndProvider() (*mockPayoutMethodRepo, *mockPaymentProvider) {
	methods := &mockPayoutMethodRepo{GetActiveFunc: func(_ context.Context, pt domain.PartnerType, pid string) (*domain.PayoutMethod, error) {
		return cardMethod(pt, pid), nil
	}}
	prov := &mockPaymentProvider{CreatePayoutFunc: func(_ context.Context, _ provider.PayoutRequest) (*provider.PayoutResult, error) {
		return &provider.PayoutResult{ProviderPayoutID: "pp1", Status: provider.PayoutStatusSucceeded}, nil
	}}
	return methods, prov
}

// On a weekday other than the configured payout day, the scheduler must not
// issue any payout even when a partner's balance is ripe.
func TestScheduler_Weekly_SkipsOffDay(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var created bool
	payouts := &mockPayoutRepo{
		CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error { created = true; return nil },
	}
	methods, prov := succeedingMethodAndProvider()

	// Compute in UTC — weeklyUseCase leaves PayoutLocation nil, which the
	// constructor defaults to UTC, so the test must not depend on the runner's TZ.
	offDay := time.Weekday((int(time.Now().UTC().Weekday()) + 1) % 7)
	uc := weeklyUseCase(payouts, methods, ripeLedger(ref, 150000), prov, offDay)
	uc.tick(context.Background())

	assert.False(t, created, "no payout may be issued on a day other than the configured weekday")
}

// On the configured weekday, a partner with no prior payout is paid.
func TestScheduler_Weekly_PaysOnConfiguredDay(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var created *domain.Payout
	payouts := &mockPayoutRepo{
		CreatePendingWithLedgerFunc: func(_ context.Context, p *domain.Payout) error { created = p; return nil },
	}
	methods, prov := succeedingMethodAndProvider()

	uc := weeklyUseCase(payouts, methods, ripeLedger(ref, 150000), prov, time.Now().UTC().Weekday())
	uc.tick(context.Background())

	assert.NotNil(t, created, "a partner with no prior payout must be paid on the configured weekday")
}

// A second payout must be suppressed for a partner already paid within the
// cooldown window — prevents re-paying funds that ripen later the same day.
func TestScheduler_Weekly_CooldownSkipsRecentlyPaid(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var created bool
	payouts := &mockPayoutRepo{
		CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error { created = true; return nil },
		LastPayoutAtFunc: func(_ context.Context, _ domain.PartnerType, _ string) (time.Time, bool, error) {
			return time.Now().Add(-2 * time.Hour), true, nil // paid earlier today
		},
	}
	methods, prov := succeedingMethodAndProvider()

	uc := weeklyUseCase(payouts, methods, ripeLedger(ref, 150000), prov, time.Now().UTC().Weekday())
	uc.tick(context.Background())

	assert.False(t, created, "a partner paid within the cooldown window must be skipped")
}

// Once the cooldown has elapsed (a full week later) the partner is paid again.
func TestScheduler_Weekly_PaysAfterCooldownElapsed(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	var created bool
	payouts := &mockPayoutRepo{
		CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error { created = true; return nil },
		LastPayoutAtFunc: func(_ context.Context, _ domain.PartnerType, _ string) (time.Time, bool, error) {
			return time.Now().Add(-7 * 24 * time.Hour), true, nil // last week
		},
	}
	methods, prov := succeedingMethodAndProvider()

	uc := weeklyUseCase(payouts, methods, ripeLedger(ref, 150000), prov, time.Now().UTC().Weekday())
	uc.tick(context.Background())

	assert.True(t, created, "a payout older than the cooldown must not block this week's run")
}

// The weekday must be evaluated in the configured timezone, not server-local /
// UTC. A far-offset zone can sit on a different calendar day than UTC; the gate
// must follow that zone's day.
func TestScheduler_Weekly_UsesConfiguredTimezone(t *testing.T) {
	t.Parallel()
	ref := domain.PartnerRef{PartnerType: domain.PartnerVenue, PartnerID: "v1"}

	loc := time.FixedZone("UTC+14", 14*3600) // Line Islands — up to a day ahead of UTC
	methods, prov := succeedingMethodAndProvider()

	newUC := func(payouts *mockPayoutRepo, weekday time.Weekday) *PayoutUseCase {
		return NewPayoutUseCase(payouts, methods, ripeLedger(ref, 150000), prov, "mock", PayoutSchedulerConfig{
			MinPayoutKopecks: 100000,
			TickInterval:     time.Hour,
			WeeklyPayout:     true,
			PayoutWeekday:    weekday,
			PayoutLocation:   loc,
		}, zerolog.Nop())
	}

	dayInLoc := time.Now().In(loc).Weekday()

	var paid bool
	p := &mockPayoutRepo{CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error { paid = true; return nil }}
	newUC(p, dayInLoc).tick(context.Background())
	assert.True(t, paid, "must pay when the configured weekday matches the day in the configured zone")

	var offPaid bool
	po := &mockPayoutRepo{CreatePendingWithLedgerFunc: func(_ context.Context, _ *domain.Payout) error { offPaid = true; return nil }}
	newUC(po, time.Weekday((int(dayInLoc)+1)%7)).tick(context.Background())
	assert.False(t, offPaid, "must skip when the configured weekday differs from the day in the configured zone")
}
