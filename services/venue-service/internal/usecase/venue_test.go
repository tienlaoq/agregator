package usecase

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
	"github.com/tienlao/agregator/services/venue-service/internal/geocode"
	"github.com/tienlao/agregator/services/venue-service/internal/repository"
)

// dummyRedis returns a client to a non-listening address so cache ops error without panicking.
func dummyRedis(t *testing.T) *goredis.Client {
	t.Helper()
	return goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:16379"})
}

func TestCreate_SetsPendingReview(t *testing.T) {
	ctx := context.Background()
	var got *domain.Venue
	repo := &mockVenueRepo{
		CreateFn: func(ctx context.Context, venue *domain.Venue) error {
			got = venue
			return nil
		},
	}
	uc := NewVenueUseCase(repo, nil)

	v := &domain.Venue{
		ID:               uuid.New(),
		OwnerID:          uuid.New(),
		Slug:             "banya-1",
		Name:             "Test",
		Type:             domain.VenueTypeBanya,
		Status:           domain.StatusPendingReview,
		LegalEntityName:  "ИП Тестов Тест Тестович",
		INN:              "7707083893",
		OGRN:             "1027700132195",
		PublicListingURL: "https://yandex.ru/maps/org/test/123",
	}
	err := uc.Create(ctx, v)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, v.ID, got.ID)
	assert.Equal(t, domain.StatusPendingReview, got.Status)
}

func TestModerate_Approve(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	moderatorID := uuid.New()
	slug := "approved-venue"

	var getCalls int
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			require.Equal(t, venueID, id)
			getCalls++
			if getCalls == 1 {
				return &domain.Venue{ID: venueID, Status: domain.StatusPendingReview, Slug: slug}, nil
			}
			return &domain.Venue{ID: venueID, Status: domain.StatusActive, Slug: slug}, nil
		},
		UpdateStatusFn: func(_ context.Context, id uuid.UUID, status, comment string, by uuid.UUID) error {
			assert.Equal(t, venueID, id)
			assert.Equal(t, domain.StatusActive, status)
			assert.Equal(t, moderatorID, by)
			return nil
		},
		InsertModerationHistoryFn: func(_ context.Context, entry *domain.ModerationHistoryEntry) error {
			assert.Equal(t, venueID, entry.VenueID)
			assert.Equal(t, domain.StatusPendingReview, entry.OldStatus)
			assert.Equal(t, domain.StatusActive, entry.NewStatus)
			assert.Equal(t, moderatorID, entry.ChangedBy)
			return nil
		},
	}

	uc := NewVenueUseCase(repo, dummyRedis(t))
	out, err := uc.Moderate(ctx, venueID, "approve", "", moderatorID)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, domain.StatusActive, out.Status)
	assert.Equal(t, 2, getCalls)
}

func TestModerate_Reject(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	moderatorID := uuid.New()
	slug := "rejected-venue"
	comment := "does not meet guidelines"

	var getCalls int
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.Venue{ID: venueID, Status: domain.StatusPendingReview, Slug: slug}, nil
			}
			return &domain.Venue{ID: venueID, Status: domain.StatusRejected, Slug: slug, ModerationComment: comment}, nil
		},
		UpdateStatusFn: func(_ context.Context, id uuid.UUID, status, c string, by uuid.UUID) error {
			assert.Equal(t, domain.StatusRejected, status)
			assert.Equal(t, comment, c)
			return nil
		},
	}

	uc := NewVenueUseCase(repo, dummyRedis(t))
	out, err := uc.Moderate(ctx, venueID, "reject", comment, moderatorID)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, domain.StatusRejected, out.Status)
}

func TestModerate_LogsHistoryInsertFailure(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	moderatorID := uuid.New()
	var logBuf bytes.Buffer
	log := zerolog.New(&logBuf)
	var getCalls int
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			require.Equal(t, venueID, id)
			getCalls++
			if getCalls == 1 {
				return &domain.Venue{ID: venueID, Status: domain.StatusPendingReview, Slug: "history-fail"}, nil
			}
			return &domain.Venue{ID: venueID, Status: domain.StatusActive, Slug: "history-fail"}, nil
		},
		UpdateStatusFn: func(_ context.Context, id uuid.UUID, status, comment string, by uuid.UUID) error {
			require.Equal(t, venueID, id)
			require.Equal(t, domain.StatusActive, status)
			require.Equal(t, moderatorID, by)
			return nil
		},
		InsertModerationHistoryFn: func(_ context.Context, entry *domain.ModerationHistoryEntry) error {
			require.Equal(t, domain.StatusPendingReview, entry.OldStatus)
			require.Equal(t, domain.StatusActive, entry.NewStatus)
			return errors.New("audit write failed")
		},
	}
	uc := NewVenueUseCaseWithConfig(repo, dummyRedis(t), log, Config{})

	out, err := uc.Moderate(ctx, venueID, "approve", "", moderatorID)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Contains(t, logBuf.String(), "moderation history insert failed")
	assert.Contains(t, logBuf.String(), "audit write failed")
	assert.Contains(t, logBuf.String(), venueID.String())
	assert.Contains(t, logBuf.String(), moderatorID.String())
}

func TestModerate_Suspend(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	moderatorID := uuid.New()
	slug := "suspended-venue"
	comment := "policy violation"

	var getCalls int
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			getCalls++
			if getCalls == 1 {
				return &domain.Venue{ID: venueID, Status: domain.StatusActive, Slug: slug}, nil
			}
			return &domain.Venue{ID: venueID, Status: domain.StatusSuspended, Slug: slug}, nil
		},
		UpdateStatusFn: func(_ context.Context, id uuid.UUID, status, c string, by uuid.UUID) error {
			assert.Equal(t, domain.StatusSuspended, status)
			assert.Equal(t, comment, c)
			return nil
		},
	}

	uc := NewVenueUseCase(repo, dummyRedis(t))
	out, err := uc.Moderate(ctx, venueID, "suspend", comment, moderatorID)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, domain.StatusSuspended, out.Status)
}

func TestModerate_Resume(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	moderatorID := uuid.New()
	slug := "was-suspended"

	var getCalls int
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			require.Equal(t, venueID, id)
			getCalls++
			if getCalls == 1 {
				return &domain.Venue{ID: venueID, Status: domain.StatusSuspended, Slug: slug}, nil
			}
			return &domain.Venue{ID: venueID, Status: domain.StatusActive, Slug: slug}, nil
		},
		UpdateStatusFn: func(_ context.Context, id uuid.UUID, status, c string, by uuid.UUID) error {
			assert.Equal(t, domain.StatusActive, status)
			assert.Equal(t, moderatorID, by)
			return nil
		},
		InsertModerationHistoryFn: func(_ context.Context, entry *domain.ModerationHistoryEntry) error {
			assert.Equal(t, domain.StatusSuspended, entry.OldStatus)
			assert.Equal(t, domain.StatusActive, entry.NewStatus)
			return nil
		},
	}

	uc := NewVenueUseCase(repo, dummyRedis(t))
	out, err := uc.Moderate(ctx, venueID, "resume", "", moderatorID)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, domain.StatusActive, out.Status)
}

func TestModerate_ResumeNotSuspended(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: id, Status: domain.StatusActive, Slug: "x"}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.Moderate(ctx, venueID, "resume", "", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "приостановлен")
}

func TestModerate_ApproveFromSuspendedFails(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: id, Status: domain.StatusSuspended, Slug: "x"}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.Moderate(ctx, venueID, "approve", "", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "одобрение")
}

func TestModerate_SuspendNotActive(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: id, Status: domain.StatusPendingReview, Slug: "x"}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.Moderate(ctx, venueID, "suspend", "reason", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "актив")
}

func TestModerate_RejectWithoutComment(t *testing.T) {
	ctx := context.Background()
	vid := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: id, Status: domain.StatusPendingReview, Slug: "x"}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.Moderate(ctx, vid, "reject", "", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "комментарий")
}

func TestModerate_SuspendWithoutComment(t *testing.T) {
	ctx := context.Background()
	vid := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: id, Status: domain.StatusActive, Slug: "x"}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.Moderate(ctx, vid, "suspend", "", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "комментарий")
}

func TestModerate_UnknownAction(t *testing.T) {
	ctx := context.Background()
	vid := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: id, Status: domain.StatusPendingReview, Slug: "x"}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.Moderate(ctx, vid, "ban", "x", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "модерации")
}

func TestModerate_VenueNotFound(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			assert.Equal(t, venueID, id)
			return nil, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.Moderate(ctx, venueID, "approve", "", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "площадка не найдена")
}

// The active/rejected → pending_review transition now rides inside repo.Update
// (a single UPDATE), so the usecase must delegate to it and never issue a
// separate reset call. The actual status transition is covered by the repository
// integration test (TestIntegration_UpdateResendsActiveToReview).
func TestUpdate_DelegatesTransitionToRepo(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	v := &domain.Venue{
		ID:     id,
		Slug:   "re-edit",
		Type:   domain.VenueTypeBanya,
		Status: domain.StatusActive,
	}

	var updateCalled, resetCalled bool
	repo := &mockVenueRepo{
		UpdateFn: func(_ context.Context, venue *domain.Venue) error {
			updateCalled = true
			assert.Equal(t, id, venue.ID)
			return nil
		},
		ResetToPendingReviewFn: func(_ context.Context, _ uuid.UUID) error {
			resetCalled = true
			return nil
		},
	}

	uc := NewVenueUseCase(repo, dummyRedis(t))
	require.NoError(t, uc.Update(ctx, v))
	assert.True(t, updateCalled)
	assert.False(t, resetCalled, "reset must be atomic inside repo.Update, not a separate call")
}

func TestOwnerMutationMethods_OwnerChangedDuringWrite(t *testing.T) {
	ctx := context.Background()
	ownerID := uuid.New()
	venueID := uuid.New()
	photoID := uuid.New()

	tests := []struct {
		name string
		run  func(*VenueUseCase) error
		repo *mockVenueRepo
	}{
		{
			name: "replace services",
			run: func(uc *VenueUseCase) error {
				return uc.ReplaceVenueServices(ctx, venueID, ownerID, []domain.VenueService{{Name: "Парение"}})
			},
			repo: &mockVenueRepo{
				ReplaceVenueServicesFn: func(_ context.Context, _, _ uuid.UUID, _ []domain.VenueService) error {
					return repository.ErrVenueOwnershipMismatch
				},
			},
		},
		{
			name: "add photo",
			run: func(uc *VenueUseCase) error {
				_, err := uc.AddVenuePhoto(ctx, venueID, ownerID, "https://example.com/photo.jpg")
				return err
			},
			repo: &mockVenueRepo{
				AddVenuePhotoFn: func(_ context.Context, _, _ uuid.UUID, _ string) (*domain.VenuePhoto, error) {
					return nil, repository.ErrVenueOwnershipMismatch
				},
			},
		},
		{
			name: "delete photo",
			run: func(uc *VenueUseCase) error {
				_, err := uc.DeleteVenuePhoto(ctx, venueID, ownerID, photoID)
				return err
			},
			repo: &mockVenueRepo{
				DeleteVenuePhotoFn: func(_ context.Context, _, _, _ uuid.UUID) (string, error) {
					return "", repository.ErrVenueOwnershipMismatch
				},
			},
		},
		{
			name: "set cover photo",
			run: func(uc *VenueUseCase) error {
				_, err := uc.SetVenueCoverPhoto(ctx, venueID, ownerID, photoID)
				return err
			},
			repo: &mockVenueRepo{
				SetVenueCoverPhotoFn: func(_ context.Context, _, _, _ uuid.UUID) error {
					return repository.ErrVenueOwnershipMismatch
				},
			},
		},
		{
			name: "replace halls",
			run: func(uc *VenueUseCase) error {
				return uc.ReplaceVenueHalls(ctx, venueID, ownerID, []domain.VenueHallUpsert{{Name: "Зал"}})
			},
			repo: &mockVenueRepo{
				ReplaceVenueHallsFn: func(_ context.Context, _, _ uuid.UUID, _ []domain.VenueHallUpsert) error {
					return repository.ErrVenueOwnershipMismatch
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.repo.GetByIDFn = func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
				assert.Equal(t, venueID, id)
				return &domain.Venue{ID: venueID, OwnerID: ownerID}, nil
			}
			uc := NewVenueUseCase(tt.repo, dummyRedis(t))

			err := tt.run(uc)
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.PermissionDenied, st.Code())
		})
	}
}

func TestUpdateRating_AllowsNilRedis(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()
	repo := &mockVenueRepo{
		UpdateRatingFn: func(_ context.Context, gotVenueID uuid.UUID, avgRating float64, reviewCount int32) error {
			assert.Equal(t, venueID, gotVenueID)
			assert.Equal(t, 4.7, avgRating)
			assert.Equal(t, int32(12), reviewCount)
			return nil
		},
	}
	uc := NewVenueUseCase(repo, nil)

	require.NoError(t, uc.UpdateRating(ctx, venueID, 4.7, 12))
}

func TestUpdateRating_Validation(t *testing.T) {
	ctx := context.Background()
	venueID := uuid.New()

	tests := []struct {
		name        string
		avgRating   float64
		reviewCount int32
		wantErr     bool
		wantCode    codes.Code
	}{
		{name: "ok_zero_rating_zero_count", avgRating: 0, reviewCount: 0},
		{name: "ok_max_rating", avgRating: 5, reviewCount: 100},
		{name: "ok_fractional", avgRating: 4.7, reviewCount: 12},
		{name: "err_rating_negative", avgRating: -0.1, reviewCount: 1, wantErr: true, wantCode: codes.InvalidArgument},
		{name: "err_rating_above_5", avgRating: 5.0001, reviewCount: 1, wantErr: true, wantCode: codes.InvalidArgument},
		{name: "err_rating_far_negative", avgRating: -100, reviewCount: 0, wantErr: true, wantCode: codes.InvalidArgument},
		{name: "err_review_count_negative", avgRating: 4.5, reviewCount: -1, wantErr: true, wantCode: codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var repoCalled bool
			repo := &mockVenueRepo{
				UpdateRatingFn: func(_ context.Context, _ uuid.UUID, _ float64, _ int32) error {
					repoCalled = true
					return nil
				},
			}
			uc := NewVenueUseCase(repo, nil)
			err := uc.UpdateRating(ctx, venueID, tt.avgRating, tt.reviewCount)
			if !tt.wantErr {
				require.NoError(t, err)
				assert.True(t, repoCalled, "repo should be called on valid input")
				return
			}
			require.Error(t, err)
			assert.False(t, repoCalled, "repo must not be called on invalid input")
			st, ok := status.FromError(err)
			require.True(t, ok, "expected gRPC status, got %T: %v", err, err)
			assert.Equal(t, tt.wantCode, st.Code())
		})
	}
}

func TestCreateManualSlotBlock_NotOwner(t *testing.T) {
	ctx := context.Background()
	owner := uuid.New()
	other := uuid.New()
	vid := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			assert.Equal(t, vid, id)
			return &domain.Venue{ID: vid, OwnerID: other}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.CreateManualSlotBlock(ctx, owner, vid, nil, "2026-01-10", "10:00", "12:00", "звонок")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestCreateManualSlotBlock_OverlapInvalidArgument(t *testing.T) {
	ctx := context.Background()
	owner := uuid.New()
	vid := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: vid, OwnerID: owner}, nil
		},
		CreateManualSlotBlockFn: func(_ context.Context, venueID uuid.UUID, _ []uuid.UUID, _, _, _, _ string) ([]domain.ManualSlotBlock, error) {
			assert.Equal(t, vid, venueID)
			return nil, repository.ErrSlotUnavailable
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.CreateManualSlotBlock(ctx, owner, vid, nil, "2026-02-01", "10:00", "12:00", "")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestDeleteManualSlotBlock_NotFound(t *testing.T) {
	ctx := context.Background()
	owner := uuid.New()
	vid := uuid.New()
	bid := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: vid, OwnerID: owner}, nil
		},
		DeleteManualSlotBlockFn: func(_ context.Context, venueID, blockID uuid.UUID) (bool, error) {
			assert.Equal(t, vid, venueID)
			assert.Equal(t, bid, blockID)
			return false, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	err := uc.DeleteManualSlotBlock(ctx, owner, vid, bid)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
}

func TestGetVenueSchedule_NotMember(t *testing.T) {
	ctx := context.Background()
	owner := uuid.New()
	other := uuid.New()
	vid := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: vid, OwnerID: owner}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.GetVenueSchedule(ctx, other, vid, "2026-01-01", "2026-01-31")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestGetVenueSchedule_DateValidation(t *testing.T) {
	ctx := context.Background()
	owner := uuid.New()
	vid := uuid.New()
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: vid, OwnerID: owner}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	cases := []struct {
		name, dateFrom, dateTo string
	}{
		{"bad_from", "nope", "2026-01-31"},
		{"bad_to", "2026-01-01", "nope"},
		{"to_before_from", "2026-02-01", "2026-01-01"},
		{"range_too_wide", "2026-01-01", "2026-12-31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uc.GetVenueSchedule(ctx, owner, vid, tc.dateFrom, tc.dateTo)
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code())
		})
	}
}

func TestGetVenueSchedule_ReturnsEntries(t *testing.T) {
	ctx := context.Background()
	owner := uuid.New()
	vid := uuid.New()
	bookingID := uuid.New()
	var gotFrom, gotTo string
	repo := &mockVenueRepo{
		GetByIDFn: func(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
			return &domain.Venue{ID: vid, OwnerID: owner}, nil
		},
		ListVenueScheduleFn: func(_ context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]domain.ScheduleEntry, error) {
			assert.Equal(t, vid, venueID)
			gotFrom, gotTo = dateFrom, dateTo
			return []domain.ScheduleEntry{
				{ID: uuid.New(), BookingID: &bookingID, Date: "2026-01-10", TimeFrom: "12:00", TimeTo: "14:00"},
				{ID: uuid.New(), Date: "2026-01-10", TimeFrom: "18:00", TimeTo: "20:00", Note: "звонок"},
			}, nil
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	entries, err := uc.GetVenueSchedule(ctx, owner, vid, "2026-01-01", "2026-01-31")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "2026-01-01", gotFrom)
	assert.Equal(t, "2026-01-31", gotTo)
	assert.NotNil(t, entries[0].BookingID)
	assert.Nil(t, entries[1].BookingID)
	assert.Equal(t, "звонок", entries[1].Note)
}

func TestReserveSlot_ModeGate(t *testing.T) {
	ctx := context.Background()
	vid := uuid.New()
	bid := uuid.New()
	hall := uuid.New()

	cases := []struct {
		name     string
		mode     string
		wantHall bool
	}{
		{"whole_ignores_halls", domain.BookingModeWhole, false},
		{"per_hall_passes_halls", domain.BookingModePerHall, true},
		{"unknown_mode_falls_back_to_whole", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotHalls []uuid.UUID
			called := false
			repo := &mockVenueRepo{
				BookingModeFn: func(_ context.Context, id uuid.UUID) (string, error) {
					assert.Equal(t, vid, id)
					return tc.mode, nil
				},
				ReserveSlotFn: func(_ context.Context, _, _ uuid.UUID, _, _, _ string, hallIDs []uuid.UUID) error {
					called = true
					gotHalls = hallIDs
					return nil
				},
			}
			uc := NewVenueUseCase(repo, dummyRedis(t))
			err := uc.ReserveSlot(ctx, vid, bid, "2026-03-01", "10:00", "12:00", []uuid.UUID{hall})
			require.NoError(t, err)
			require.True(t, called)
			if tc.wantHall {
				assert.Equal(t, []uuid.UUID{hall}, gotHalls)
			} else {
				assert.Nil(t, gotHalls)
			}
		})
	}
}

func TestReserveSlot_PerHallRequiresHall(t *testing.T) {
	ctx := context.Background()
	vid := uuid.New()
	bid := uuid.New()
	hall := uuid.New()

	t.Run("per_hall_no_hall_with_halls_rejected", func(t *testing.T) {
		reserved := false
		repo := &mockVenueRepo{
			BookingModeFn: func(_ context.Context, _ uuid.UUID) (string, error) { return domain.BookingModePerHall, nil },
			HallIDsFn:     func(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) { return []uuid.UUID{hall}, nil },
			ReserveSlotFn: func(_ context.Context, _, _ uuid.UUID, _, _, _ string, _ []uuid.UUID) error {
				reserved = true
				return nil
			},
		}
		uc := NewVenueUseCase(repo, dummyRedis(t))
		err := uc.ReserveSlot(ctx, vid, bid, "2026-03-01", "10:00", "12:00", nil)
		require.Error(t, err)
		assert.False(t, reserved, "must not reserve a whole-venue row in per_hall")
	})

	t.Run("per_hall_no_hall_no_halls_allowed", func(t *testing.T) {
		var gotHalls []uuid.UUID
		repo := &mockVenueRepo{
			BookingModeFn: func(_ context.Context, _ uuid.UUID) (string, error) { return domain.BookingModePerHall, nil },
			HallIDsFn:     func(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) { return nil, nil },
			ReserveSlotFn: func(_ context.Context, _, _ uuid.UUID, _, _, _ string, hallIDs []uuid.UUID) error {
				gotHalls = hallIDs
				return nil
			},
		}
		uc := NewVenueUseCase(repo, dummyRedis(t))
		err := uc.ReserveSlot(ctx, vid, bid, "2026-03-01", "10:00", "12:00", nil)
		require.NoError(t, err)
		assert.Nil(t, gotHalls, "no halls defined → whole-venue reservation")
	})
}

func TestCheckSlot_ModeGate(t *testing.T) {
	ctx := context.Background()
	vid := uuid.New()
	hall := uuid.New()

	cases := []struct {
		name      string
		mode      string
		wantHalls []uuid.UUID
	}{
		{"whole_ignores_halls", domain.BookingModeWhole, nil},
		{"per_hall_passes_halls", domain.BookingModePerHall, []uuid.UUID{hall}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []uuid.UUID
			repo := &mockVenueRepo{
				BookingModeFn: func(_ context.Context, _ uuid.UUID) (string, error) { return tc.mode, nil },
				CheckSlotFn: func(_ context.Context, _ uuid.UUID, hallIDs []uuid.UUID, _, _, _ string) (bool, error) {
					got = hallIDs
					return true, nil
				},
			}
			uc := NewVenueUseCase(repo, dummyRedis(t))
			ok, err := uc.CheckSlot(ctx, vid, []uuid.UUID{hall}, "2026-03-01", "10:00", "12:00")
			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, tc.wantHalls, got)
		})
	}
}

func TestSetBookingMode(t *testing.T) {
	ctx := context.Background()
	owner := uuid.New()
	vid := uuid.New()
	newRepo := func(set func(context.Context, uuid.UUID, string) error) *mockVenueRepo {
		return &mockVenueRepo{
			GetByIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Venue, error) {
				return &domain.Venue{ID: vid, OwnerID: owner}, nil
			},
			SetBookingModeFn: set,
		}
	}

	t.Run("valid_per_hall", func(t *testing.T) {
		var gotMode string
		uc := NewVenueUseCase(newRepo(func(_ context.Context, _ uuid.UUID, mode string) error {
			gotMode = mode
			return nil
		}), dummyRedis(t))
		require.NoError(t, uc.SetBookingMode(ctx, owner, vid, domain.BookingModePerHall))
		assert.Equal(t, domain.BookingModePerHall, gotMode)
	})

	t.Run("invalid_mode_rejected", func(t *testing.T) {
		called := false
		uc := NewVenueUseCase(newRepo(func(_ context.Context, _ uuid.UUID, _ string) error {
			called = true
			return nil
		}), dummyRedis(t))
		err := uc.SetBookingMode(ctx, owner, vid, "nonsense")
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.False(t, called)
	})

	t.Run("not_member", func(t *testing.T) {
		uc := NewVenueUseCase(newRepo(func(_ context.Context, _ uuid.UUID, _ string) error { return nil }), dummyRedis(t))
		err := uc.SetBookingMode(ctx, uuid.New(), vid, domain.BookingModePerHall)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.PermissionDenied, st.Code())
	})

	t.Run("upcoming_reservations_rejected", func(t *testing.T) {
		uc := NewVenueUseCase(newRepo(func(_ context.Context, _ uuid.UUID, _ string) error {
			return repository.ErrBookingModeHasReservations
		}), dummyRedis(t))
		err := uc.SetBookingMode(ctx, owner, vid, domain.BookingModePerHall)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.FailedPrecondition, st.Code())
	})
}

func TestCreateManualSlotBlock_HallResolution(t *testing.T) {
	ctx := context.Background()
	owner := uuid.New()
	vid := uuid.New()
	hallA, hallB := uuid.New(), uuid.New()

	base := func(mode string, capture *[]uuid.UUID) *mockVenueRepo {
		return &mockVenueRepo{
			GetByIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Venue, error) {
				return &domain.Venue{ID: vid, OwnerID: owner}, nil
			},
			BookingModeFn: func(_ context.Context, _ uuid.UUID) (string, error) { return mode, nil },
			HallIDsFn: func(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
				return []uuid.UUID{hallA, hallB}, nil
			},
			CreateManualSlotBlockFn: func(_ context.Context, _ uuid.UUID, hallIDs []uuid.UUID, _, _, _, _ string) ([]domain.ManualSlotBlock, error) {
				*capture = hallIDs
				return []domain.ManualSlotBlock{{ID: uuid.New()}}, nil
			},
		}
	}

	t.Run("whole_ignores_halls", func(t *testing.T) {
		var got []uuid.UUID
		uc := NewVenueUseCase(base(domain.BookingModeWhole, &got), dummyRedis(t))
		_, err := uc.CreateManualSlotBlock(ctx, owner, vid, []uuid.UUID{hallA}, "2026-03-01", "10:00", "12:00", "")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("per_hall_empty_rejected", func(t *testing.T) {
		var got []uuid.UUID
		uc := NewVenueUseCase(base(domain.BookingModePerHall, &got), dummyRedis(t))
		_, err := uc.CreateManualSlotBlock(ctx, owner, vid, nil, "2026-03-01", "10:00", "12:00", "")
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		assert.Nil(t, got, "must not create any block row")
	})

	t.Run("per_hall_specific_hall", func(t *testing.T) {
		var got []uuid.UUID
		uc := NewVenueUseCase(base(domain.BookingModePerHall, &got), dummyRedis(t))
		_, err := uc.CreateManualSlotBlock(ctx, owner, vid, []uuid.UUID{hallB}, "2026-03-01", "10:00", "12:00", "")
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{hallB}, got)
	})

	t.Run("per_hall_unknown_hall_rejected", func(t *testing.T) {
		var got []uuid.UUID
		uc := NewVenueUseCase(base(domain.BookingModePerHall, &got), dummyRedis(t))
		_, err := uc.CreateManualSlotBlock(ctx, owner, vid, []uuid.UUID{uuid.New()}, "2026-03-01", "10:00", "12:00", "")
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("per_hall_no_halls_defined_falls_back_to_whole", func(t *testing.T) {
		got := []uuid.UUID{uuid.New()} // sentinel: must be overwritten with nil
		repo := &mockVenueRepo{
			GetByIDFn: func(_ context.Context, _ uuid.UUID) (*domain.Venue, error) {
				return &domain.Venue{ID: vid, OwnerID: owner}, nil
			},
			BookingModeFn: func(_ context.Context, _ uuid.UUID) (string, error) { return domain.BookingModePerHall, nil },
			HallIDsFn:     func(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) { return nil, nil },
			CreateManualSlotBlockFn: func(_ context.Context, _ uuid.UUID, hallIDs []uuid.UUID, _, _, _, _ string) ([]domain.ManualSlotBlock, error) {
				got = hallIDs
				return []domain.ManualSlotBlock{{ID: uuid.New()}}, nil
			},
		}
		uc := NewVenueUseCase(repo, dummyRedis(t))
		_, err := uc.CreateManualSlotBlock(ctx, owner, vid, nil, "2026-03-01", "10:00", "12:00", "")
		require.NoError(t, err)
		assert.Nil(t, got, "no halls → whole-venue block")
	})
}

// Гео-ключ кеша бакетится: без этого каждое GPS-чтение — уникальный ключ,
// и «бани рядом» промахивается мимо кеша на каждом запросе.
func TestSearchParamsCacheString_GeoBucketing(t *testing.T) {
	base := domain.SearchParams{Lat: 55.796000, Lng: 49.106000, RadiusKM: 10, Page: 1, PageSize: 12}

	tests := []struct {
		name      string
		other     domain.SearchParams
		wantEqual bool
	}{
		{
			name:      "shift within ~110m shares a key",
			other:     domain.SearchParams{Lat: 55.796210, Lng: 49.106180, RadiusKM: 10, Page: 1, PageSize: 12},
			wantEqual: true,
		},
		{
			name:      "shift of ~1km gets its own key",
			other:     domain.SearchParams{Lat: 55.805000, Lng: 49.106000, RadiusKM: 10, Page: 1, PageSize: 12},
			wantEqual: false,
		},
		{
			name:      "different radius at same point gets its own key",
			other:     domain.SearchParams{Lat: 55.796000, Lng: 49.106000, RadiusKM: 25, Page: 1, PageSize: 12},
			wantEqual: false,
		},
		{
			name:      "different page gets its own key",
			other:     domain.SearchParams{Lat: 55.796000, Lng: 49.106000, RadiusKM: 10, Page: 2, PageSize: 12},
			wantEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := searchParamsCacheString(1, base) == searchParamsCacheString(1, tt.other)
			assert.Equal(t, tt.wantEqual, got)
		})
	}
}

// stubGeocoder implements the usecase's geocoder interface with a function hook.
type stubGeocoder struct {
	calls int
	fn    func(ctx context.Context, address string) (geocode.Point, error)
}

func (s *stubGeocoder) Geocode(ctx context.Context, address string) (geocode.Point, error) {
	s.calls++
	return s.fn(ctx, address)
}

func geocodableVenue() *domain.Venue {
	return &domain.Venue{
		ID:               uuid.New(),
		OwnerID:          uuid.New(),
		Slug:             "banya-geo",
		Name:             "Гео",
		Type:             domain.VenueTypeBanya,
		City:             "Казань",
		Address:          "ул. Баумана, 5",
		LegalEntityName:  "ИП Тестов Тест Тестович",
		INN:              "7707083893",
		OGRN:             "1027700132195",
		PublicListingURL: "https://yandex.ru/maps/org/test/123",
	}
}

func TestCreate_GeocodesAddress(t *testing.T) {
	kazan := geocode.Point{Lat: 55.796127, Lng: 49.106414}

	tests := []struct {
		name          string
		venue         func() *domain.Venue
		geo           func(ctx context.Context, address string) (geocode.Point, error)
		wantCalls     int
		wantLat       float64
		wantLng       float64
		wantQuery     string
		wantCreateErr bool
	}{
		{
			name:      "empty coordinates are filled from city + address",
			venue:     geocodableVenue,
			geo:       func(context.Context, string) (geocode.Point, error) { return kazan, nil },
			wantCalls: 1,
			wantLat:   kazan.Lat,
			wantLng:   kazan.Lng,
			wantQuery: "Казань, ул. Баумана, 5",
		},
		{
			name: "caller-supplied point is never overwritten",
			venue: func() *domain.Venue {
				v := geocodableVenue()
				v.Latitude, v.Longitude = 55.75, 37.61
				return v
			},
			geo:       func(context.Context, string) (geocode.Point, error) { return kazan, nil },
			wantCalls: 0,
			wantLat:   55.75,
			wantLng:   37.61,
		},
		{
			name:      "geocoder failure still creates the venue",
			venue:     geocodableVenue,
			geo:       func(context.Context, string) (geocode.Point, error) { return geocode.Point{}, geocode.ErrNotFound },
			wantCalls: 1,
		},
		{
			name: "no address, no call",
			venue: func() *domain.Venue {
				v := geocodableVenue()
				v.Address = "   "
				return v
			},
			geo:       func(context.Context, string) (geocode.Point, error) { return kazan, nil },
			wantCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *domain.Venue
			repo := &mockVenueRepo{
				CreateFn: func(ctx context.Context, venue *domain.Venue) error {
					got = venue
					return nil
				},
			}
			var gotQuery string
			geo := &stubGeocoder{fn: func(ctx context.Context, address string) (geocode.Point, error) {
				gotQuery = address
				return tt.geo(ctx, address)
			}}
			uc := NewVenueUseCaseWithConfig(repo, nil, zerolog.Nop(), Config{Geocoder: geo})

			err := uc.Create(context.Background(), tt.venue())

			require.NoError(t, err, "geocoding must never fail the write")
			require.NotNil(t, got, "venue must be persisted regardless of geocoding")
			assert.Equal(t, tt.wantCalls, geo.calls)
			assert.Equal(t, tt.wantLat, got.Latitude)
			assert.Equal(t, tt.wantLng, got.Longitude)
			if tt.wantQuery != "" {
				assert.Equal(t, tt.wantQuery, gotQuery, "city must scope the query")
			}
		})
	}
}

func TestCreate_WithoutGeocoderKeepsWorking(t *testing.T) {
	var got *domain.Venue
	repo := &mockVenueRepo{CreateFn: func(ctx context.Context, venue *domain.Venue) error {
		got = venue
		return nil
	}}
	uc := NewVenueUseCase(repo, nil) // no geocoder configured

	require.NoError(t, uc.Create(context.Background(), geocodableVenue()))
	require.NotNil(t, got)
	assert.Zero(t, got.Latitude)
	assert.Zero(t, got.Longitude)
}
