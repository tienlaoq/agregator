package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/venue-service/internal/domain"
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
		Status:           domain.StatusPendingReview,
		LegalEntityName:  "ИП Тестов Тест Тестович",
		INN:              "7707083893",
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

func TestModerate_RejectWithoutComment(t *testing.T) {
	ctx := context.Background()
	uc := NewVenueUseCase(&mockVenueRepo{}, dummyRedis(t))
	_, err := uc.Moderate(ctx, uuid.New(), "reject", "", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "comment is required")
}

func TestModerate_SuspendWithoutComment(t *testing.T) {
	ctx := context.Background()
	uc := NewVenueUseCase(&mockVenueRepo{}, dummyRedis(t))
	_, err := uc.Moderate(ctx, uuid.New(), "suspend", "", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "comment is required")
}

func TestModerate_UnknownAction(t *testing.T) {
	ctx := context.Background()
	uc := NewVenueUseCase(&mockVenueRepo{}, dummyRedis(t))
	_, err := uc.Moderate(ctx, uuid.New(), "ban", "x", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown moderation action")
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
	assert.Contains(t, err.Error(), "venue not found")
}

func TestUpdate_RejectedResetsToReview(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	v := &domain.Venue{
		ID:     id,
		Slug:   "re-edit",
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
}
