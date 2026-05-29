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
	uc := NewVenueUseCaseWithLogger(repo, dummyRedis(t), log)

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

func TestUpdate_RejectedResetsToReview(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	v := &domain.Venue{
		ID:     id,
		Slug:   "re-edit",
		Type:   domain.VenueTypeBanya,
		Status: domain.StatusRejected,
	}

	var resetCalled bool
	repo := &mockVenueRepo{
		UpdateFn: func(_ context.Context, venue *domain.Venue) error {
			assert.Equal(t, id, venue.ID)
			return nil
		},
		ResetToPendingReviewFn: func(_ context.Context, venueID uuid.UUID) error {
			resetCalled = true
			assert.Equal(t, id, venueID)
			return nil
		},
	}

	uc := NewVenueUseCase(repo, dummyRedis(t))
	err := uc.Update(ctx, v)
	require.NoError(t, err)
	assert.True(t, resetCalled)
	assert.Equal(t, domain.StatusPendingReview, v.Status)
	assert.False(t, v.IsActive)
}

func TestUpdate_ActiveResetsToReview(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	v := &domain.Venue{
		ID:       id,
		Slug:     "active-edit",
		Type:     domain.VenueTypeBanya,
		Status:   domain.StatusActive,
		IsActive: true,
	}

	var resetCalled bool
	repo := &mockVenueRepo{
		UpdateFn: func(_ context.Context, venue *domain.Venue) error { return nil },
		ResetToPendingReviewFn: func(_ context.Context, venueID uuid.UUID) error {
			resetCalled = true
			assert.Equal(t, id, venueID)
			return nil
		},
	}

	uc := NewVenueUseCase(repo, dummyRedis(t))
	err := uc.Update(ctx, v)
	require.NoError(t, err)
	assert.True(t, resetCalled)
	assert.Equal(t, domain.StatusPendingReview, v.Status)
	assert.False(t, v.IsActive)
}

func TestUpdate_PendingStaysPending(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	v := &domain.Venue{
		ID:     id,
		Slug:   "pending-edit",
		Type:   domain.VenueTypeBanya,
		Status: domain.StatusPendingReview,
	}

	var resetCalled bool
	repo := &mockVenueRepo{
		UpdateFn: func(_ context.Context, venue *domain.Venue) error { return nil },
		ResetToPendingReviewFn: func(_ context.Context, venueID uuid.UUID) error {
			resetCalled = true
			return nil
		},
	}

	uc := NewVenueUseCase(repo, dummyRedis(t))
	err := uc.Update(ctx, v)
	require.NoError(t, err)
	assert.False(t, resetCalled)
	assert.Equal(t, domain.StatusPendingReview, v.Status)
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
	_, err := uc.CreateManualSlotBlock(ctx, owner, vid, "2026-01-10", "10:00", "12:00", "звонок")
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
		CreateManualSlotBlockFn: func(_ context.Context, venueID uuid.UUID, _, _, _, _ string) (uuid.UUID, error) {
			assert.Equal(t, vid, venueID)
			return uuid.Nil, repository.ErrSlotUnavailable
		},
	}
	uc := NewVenueUseCase(repo, dummyRedis(t))
	_, err := uc.CreateManualSlotBlock(ctx, owner, vid, "2026-02-01", "10:00", "12:00", "")
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
