//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/master-service/internal/domain"
)

func TestInsertAndGet_RoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusDraft)

	byID, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	require.Equal(t, m.UserID, byID.UserID)
	require.Equal(t, m.Slug, byID.Slug)
	require.Equal(t, domain.StatusDraft, byID.Status)

	byUser, err := repo.GetByUserID(ctx, m.UserID)
	require.NoError(t, err)
	require.Equal(t, m.ID, byUser.ID)

	bySlug, err := repo.GetBySlug(ctx, m.Slug)
	require.NoError(t, err)
	require.Equal(t, m.ID, bySlug.ID)
}

func TestInsert_DuplicateUserID(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusDraft)

	dup := &domain.Master{
		ID:                       uuid.New(),
		UserID:                   m.UserID, // collides with the seeded profile
		Slug:                     "master-" + uuid.NewString()[:12],
		WorkFormat:               domain.WorkFormatBoth,
		Specializations:          []string{},
		AvailabilityJSON:         "{}",
		PayoutVerificationStatus: domain.PayoutVerificationUnverified,
		Status:                   domain.StatusDraft,
	}
	err := repo.Insert(ctx, dup)
	require.ErrorIs(t, err, domain.ErrUserProfileExists)
}

func TestInsert_DuplicateSlug(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusDraft)

	dup := &domain.Master{
		ID:                       uuid.New(),
		UserID:                   uuid.New(),
		Slug:                     m.Slug, // collides
		WorkFormat:               domain.WorkFormatBoth,
		Specializations:          []string{},
		AvailabilityJSON:         "{}",
		PayoutVerificationStatus: domain.PayoutVerificationUnverified,
		Status:                   domain.StatusDraft,
	}
	err := repo.Insert(ctx, dup)
	require.ErrorIs(t, err, domain.ErrSlugConflict)
}

func TestSubmitForReviewAtomic(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusDraft)

	require.NoError(t, repo.SubmitForReviewAtomic(ctx, m.ID, m.UserID))

	got, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPendingReview, got.Status)

	// A history entry was recorded for the transition.
	hist, err := repo.ListModerationHistory(ctx, m.ID, 10)
	require.NoError(t, err)
	require.Len(t, hist, 1)
	require.Equal(t, domain.StatusDraft, hist[0].OldStatus)
	require.Equal(t, domain.StatusPendingReview, hist[0].NewStatus)

	// Second submit from pending_review is not allowed (duplicate-submit guard).
	err = repo.SubmitForReviewAtomic(ctx, m.ID, m.UserID)
	require.ErrorIs(t, err, domain.ErrSubmitNotAllowed)
}

func TestModerateAtomic_HappyAndConflict(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusPendingReview)
	moderator := uuid.New()

	// Conflict: expected old status does not match the current row.
	err := repo.ModerateAtomic(ctx, m.ID, domain.StatusActive, domain.StatusActive, "x", &moderator)
	require.ErrorIs(t, err, domain.ErrModerationConflict)

	// Happy path: pending_review → active with the correct expected status.
	require.NoError(t, repo.ModerateAtomic(ctx, m.ID, domain.StatusPendingReview, domain.StatusActive, "approved", &moderator))

	got, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusActive, got.Status)
	require.Equal(t, "approved", got.ModerationComment)
	require.NotNil(t, got.ModeratedBy)
	require.Equal(t, moderator, *got.ModeratedBy)
}

func TestModerateAtomic_NotFound(t *testing.T) {
	ctx := context.Background()
	moderator := uuid.New()
	err := newRepo().ModerateAtomic(ctx, uuid.New(), domain.StatusPendingReview, domain.StatusActive, "x", &moderator)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListByStatus(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	// Use a unique moderation comment as a marker so we can count only our rows.
	m := seedMaster(ctx, t, domain.StatusPendingReview)

	list, total, err := repo.ListByStatus(ctx, domain.StatusPendingReview, 100, 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int32(1))

	var found bool
	for i := range list {
		if list[i].ID == m.ID {
			found = true
		}
	}
	require.True(t, found, "seeded pending_review master must appear in ListByStatus")
}

func TestBookingPaymentFlow(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusActive)

	b := &domain.MasterBooking{
		MasterID:     m.ID,
		ClientUserID: uuid.New(),
		Date:         "2026-07-01",
		TimeFrom:     "10:00",
		TimeTo:       "11:00",
		Status:       domain.BookingStatusPending,
	}
	require.NoError(t, repo.InsertBooking(ctx, b))
	require.NotEqual(t, uuid.Nil, b.ID)

	// Move to payment_pending (as the CreateBooking saga does after CreatePayment).
	require.NoError(t, repo.SetBookingPayment(ctx, b.ID, "pay-1", "https://pay", 300000, domain.BookingStatusPaymentPending))

	// payment.completed → confirmed.
	require.NoError(t, repo.ConfirmBookingByPayment(ctx, b.ID, "pay-1"))

	got, err := repo.GetBookingByID(ctx, b.ID)
	require.NoError(t, err)
	require.Equal(t, domain.BookingStatusConfirmed, got.Status)

	// Re-confirming is idempotent (already confirmed, same payment id).
	require.NoError(t, repo.ConfirmBookingByPayment(ctx, b.ID, "pay-1"))

	// Cancelling a now-confirmed booking is a terminal-state no-op.
	err = repo.CancelBookingByPayment(ctx, b.ID, "pay-1")
	require.ErrorIs(t, err, domain.ErrBookingNotPending)
}

func TestConfirmBookingByPayment_NotFound(t *testing.T) {
	ctx := context.Background()
	err := newRepo().ConfirmBookingByPayment(ctx, uuid.New(), "pay-x")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestConfirmBookingByPayment_Mismatch(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusActive)
	b := &domain.MasterBooking{
		MasterID: m.ID, ClientUserID: uuid.New(),
		Date: "2026-07-02", TimeFrom: "12:00", TimeTo: "13:00",
		Status: domain.BookingStatusPending,
	}
	require.NoError(t, repo.InsertBooking(ctx, b))
	require.NoError(t, repo.SetBookingPayment(ctx, b.ID, "pay-A", "", 100, domain.BookingStatusPaymentPending))

	// A different payment id must not confirm a booking already bound to pay-A.
	err := repo.ConfirmBookingByPayment(ctx, b.ID, "pay-B")
	require.ErrorIs(t, err, domain.ErrPaymentMismatch)
}

func TestPhotoLimitEnforced(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusDraft)

	for i := int32(0); i < domain.MaxMasterPhotos; i++ {
		p, err := repo.AddMasterPhoto(ctx, m.ID, "/photo.jpg")
		require.NoError(t, err)
		require.Equal(t, i == 0, p.IsCover, "first photo must be cover, the rest must not")
	}

	cnt, err := repo.CountPhotosByMaster(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, domain.MaxMasterPhotos, cnt)

	_, err = repo.AddMasterPhoto(ctx, m.ID, "/over-limit.jpg")
	require.ErrorIs(t, err, domain.ErrPhotoLimitReached)
}

func TestNewSlug_DerivesFromNameWithUniqueSuffix(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()

	// NewSlug transliterates the display name and appends random entropy, so the
	// same name yields a stable prefix but distinct full slugs on each call.
	s1, err := repo.NewSlug(ctx, "Иван Банщик", uuid.New())
	require.NoError(t, err)
	require.NotEmpty(t, s1)

	s2, err := repo.NewSlug(ctx, "Иван Банщик", uuid.New())
	require.NoError(t, err)
	require.NotEqual(t, s1, s2, "random suffix must make repeated slugs distinct")

	// The inserted slug round-trips through the unique slug column.
	m := &domain.Master{
		ID: uuid.New(), UserID: uuid.New(), Slug: s1,
		WorkFormat: domain.WorkFormatBoth, Specializations: []string{},
		AvailabilityJSON:         "{}",
		PayoutVerificationStatus: domain.PayoutVerificationUnverified, Status: domain.StatusDraft,
	}
	require.NoError(t, repo.Insert(ctx, m))
	got, err := repo.GetBySlug(ctx, s1)
	require.NoError(t, err)
	require.Equal(t, m.ID, got.ID)
}

func TestUpdateProfile(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusDraft)

	m.DisplayName = "Обновлённое Имя"
	m.City = "Казань"
	m.Bio = "новое био"
	require.NoError(t, repo.UpdateProfile(ctx, m))

	got, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.Equal(t, "Обновлённое Имя", got.DisplayName)
	require.Equal(t, "Казань", got.City)
	require.Equal(t, "новое био", got.Bio)
}

// TestListPublic_FullTextSearch validates migration 011: ListPublic must match
// via masters_search_tsv / idx_masters_fts. The unique display name keeps the
// assertion immune to other rows in the shared container.
func TestListPublic_FullTextSearch(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	marker := "Парильщик" + uuid.NewString()[:8]

	m := seedMaster(ctx, t, domain.StatusActive)
	m.DisplayName = marker
	require.NoError(t, repo.UpdateProfile(ctx, m))

	list, total, err := repo.ListPublic(ctx, domain.ListPublicMastersParams{Query: marker, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int32(1), total)
	require.Len(t, list, 1)
	require.Equal(t, m.ID, list[0].ID)
}

// TestAvailabilityJSON_RoundTrip validates migration 017: the column is JSONB
// and round-trips a real object; invalid JSON is rejected by the type/CHECK.
func TestAvailabilityJSON_RoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusDraft)

	m.AvailabilityJSON = `{"mon": ["10:00-14:00"], "tue": []}`
	require.NoError(t, repo.UpdateProfile(ctx, m))

	got, err := repo.GetByID(ctx, m.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{"mon": ["10:00-14:00"], "tue": []}`, got.AvailabilityJSON)

	// The JSONB column + CHECK rejects a non-object value written directly.
	_, err = testPool.Exec(ctx,
		`UPDATE masters SET availability_json = '[]'::jsonb WHERE id = $1`, m.ID)
	require.Error(t, err, "availability_json CHECK must reject a non-object")
}

// minEffectivePrice reads the denormalised sort column directly (not exposed on
// domain.Master).
func minEffectivePrice(ctx context.Context, t *testing.T, masterID uuid.UUID) int64 {
	t.Helper()
	var v *int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT min_effective_price_kopecks FROM masters WHERE id = $1`, masterID).Scan(&v))
	require.NotNil(t, v, "min_effective_price_kopecks should be populated by the trigger")
	return *v
}

// TestMinPrice_UpdateTriggerRecomputes validates migration 019: the statement-
// level UPDATE trigger (previously rejected by PG) now exists and recomputes the
// denormalised min price when a service's price changes via a real UPDATE.
func TestMinPrice_UpdateTriggerRecomputes(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	m := seedMaster(ctx, t, domain.StatusDraft)

	// INSERT trigger path: ReplaceServices inserts one service at 500_000.
	svcs, err := repo.ReplaceServices(ctx, m.ID, []domain.MasterServiceUpsert{
		{Name: "Парение", Price: 500000, DurationMin: 60},
	})
	require.NoError(t, err)
	require.Len(t, svcs, 1)
	require.Equal(t, int64(500000), minEffectivePrice(ctx, t, m.ID))

	// UPDATE trigger path: lower the price via a direct UPDATE on master_services.
	_, err = testPool.Exec(ctx,
		`UPDATE master_services SET price = 300000 WHERE id = $1`, svcs[0].ID)
	require.NoError(t, err)
	require.Equal(t, int64(300000), minEffectivePrice(ctx, t, m.ID),
		"UPDATE trigger (migration 019) must recompute min price")
}
