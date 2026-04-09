package usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
	"github.com/tienlao/agregator/services/review-service/internal/events"
)

func TestCreateReview_InvalidRating(t *testing.T) {
	ctx := context.Background()
	uc := NewReviewUseCase(&mockReviewRepo{}, &mockBookingClient{}, &mockVenueClient{}, events.NewPublisher(nil))

	for _, rating := range []int32{0, 6, -1} {
		t.Run(fmt.Sprintf("rating_%d", rating), func(t *testing.T) {
			_, err := uc.CreateReview(ctx, CreateReviewInput{
				UserID:  "u",
				VenueID: "v",
				Rating:  rating,
				Text:    "x",
			})
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code())
			assert.Contains(t, st.Message(), "rating must be between 1 and 5")
		})
	}
}

func TestCreateReview_Success(t *testing.T) {
	ctx := context.Background()
	var gotCreate *domain.Review
	var bookingReq *bookingv1.HasCompletedBookingRequest
	var updateVenueRatingCalls []string

	repo := &mockReviewRepo{
		CreateFunc: func(ctx context.Context, review *domain.Review) error {
			gotCreate = review
			review.ID = "review-id-1"
			return nil
		},
		UpdateVenueRatingFunc: func(ctx context.Context, venueID string) error {
			updateVenueRatingCalls = append(updateVenueRatingCalls, venueID)
			return nil
		},
	}
	bookingClient := &mockBookingClient{
		HasCompletedBookingFunc: func(ctx context.Context, in *bookingv1.HasCompletedBookingRequest, opts ...grpc.CallOption) (*bookingv1.HasCompletedBookingResponse, error) {
			bookingReq = in
			return &bookingv1.HasCompletedBookingResponse{HasCompleted: true}, nil
		},
	}

	uc := NewReviewUseCase(repo, bookingClient, &mockVenueClient{}, events.NewPublisher(nil))

	var panicked any
	func() {
		defer func() { panicked = recover() }()
		_, _ = uc.CreateReview(ctx, CreateReviewInput{
			UserID:  "user-1",
			VenueID: "venue-1",
			Rating:  4,
			Text:    "great",
		})
	}()

	require.NotNil(t, panicked, "nil JetStreamContext: PublishReviewCreated panics after repo work")
	require.NotNil(t, bookingReq)
	assert.Equal(t, "user-1", bookingReq.UserId)
	assert.Equal(t, "venue-1", bookingReq.VenueId)
	require.NotNil(t, gotCreate)
	assert.Equal(t, "user-1", gotCreate.UserID)
	assert.Equal(t, "venue-1", gotCreate.VenueID)
	assert.Equal(t, int32(4), gotCreate.Rating)
	assert.Equal(t, "great", gotCreate.Text)
	assert.True(t, gotCreate.IsVerified)
	assert.Equal(t, "review-id-1", gotCreate.ID)
	assert.Equal(t, []string{"venue-1"}, updateVenueRatingCalls)
}

func TestListVenueReviews(t *testing.T) {
	ctx := context.Background()
	want := []*domain.Review{{ID: "r1", VenueID: "v1"}}
	repo := &mockReviewRepo{
		ListByVenueFunc: func(ctx context.Context, venueID string, page, pageSize int32) ([]*domain.Review, int32, error) {
			assert.Equal(t, "v1", venueID)
			assert.Equal(t, int32(2), page)
			assert.Equal(t, int32(10), pageSize)
			return want, 99, nil
		},
	}
	uc := NewReviewUseCase(repo, &mockBookingClient{}, &mockVenueClient{}, nil)

	got, total, err := uc.ListVenueReviews(ctx, "v1", 2, 10)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, int32(99), total)
}

func TestGetReview(t *testing.T) {
	ctx := context.Background()
	want := &domain.Review{ID: "r1", Text: "ok"}
	repo := &mockReviewRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.Review, error) {
			assert.Equal(t, "r1", id)
			return want, nil
		},
	}
	uc := NewReviewUseCase(repo, &mockBookingClient{}, &mockVenueClient{}, nil)

	got, err := uc.GetReview(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGetVenueRating(t *testing.T) {
	ctx := context.Background()
	want := &domain.VenueRating{VenueID: "v1", AvgRating: 4.5, ReviewCount: 3, UpdatedAt: time.Unix(1, 0).UTC()}
	repo := &mockReviewRepo{
		GetVenueRatingFunc: func(ctx context.Context, venueID string) (*domain.VenueRating, error) {
			assert.Equal(t, "v1", venueID)
			return want, nil
		},
	}
	uc := NewReviewUseCase(repo, &mockBookingClient{}, &mockVenueClient{}, nil)

	got, err := uc.GetVenueRating(ctx, "v1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
