package usecase

import (
	"context"
	"fmt"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
	"github.com/tienlao/agregator/services/review-service/internal/events"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ReviewUseCase struct {
	repo          domain.ReviewRepository
	bookingClient bookingv1.BookingServiceClient
	venueClient   venuev1.VenueServiceClient
	masterClient  masterv1.MasterServiceClient
	publisher     *events.Publisher
}

func NewReviewUseCase(
	repo domain.ReviewRepository,
	bookingClient bookingv1.BookingServiceClient,
	venueClient venuev1.VenueServiceClient,
	masterClient masterv1.MasterServiceClient,
	publisher *events.Publisher,
) *ReviewUseCase {
	return &ReviewUseCase{
		repo:          repo,
		bookingClient: bookingClient,
		venueClient:   venueClient,
		masterClient:  masterClient,
		publisher:     publisher,
	}
}

type CreateReviewInput struct {
	UserID      string
	UserName    string
	VenueID     string
	MasterID    string
	Rating      int32
	Text        string
	IsAnonymous bool
}

func (uc *ReviewUseCase) CreateReview(ctx context.Context, in CreateReviewInput) (*domain.Review, error) {
	if in.Rating < 1 || in.Rating > 5 {
		return nil, pkgerr.InvalidArgument("rating must be between 1 and 5")
	}
	hasVenue := in.VenueID != ""
	hasMaster := in.MasterID != ""
	if hasVenue == hasMaster {
		return nil, pkgerr.InvalidArgument("exactly one target is required: venue_id or master_id")
	}

	isVerified := false
	if hasVenue {
		hasVisited, err := uc.bookingClient.HasCompletedBooking(ctx, &bookingv1.HasCompletedBookingRequest{
			UserId:  in.UserID,
			VenueId: in.VenueID,
		})
		if err != nil {
			return nil, fmt.Errorf("check completed booking: %w", err)
		}
		isVerified = hasVisited.GetHasCompleted()
	} else if hasMaster {
		if uc.masterClient != nil {
			hasVisited, err := uc.masterClient.HasCompletedMasterBooking(ctx, &masterv1.HasCompletedMasterBookingRequest{
				ClientUserId: in.UserID,
				MasterId:     in.MasterID,
			})
			if err != nil {
				return nil, fmt.Errorf("check completed master booking: %w", err)
			}
			isVerified = hasVisited.GetHasCompleted()
		}
	}
	if !isVerified {
		return nil, status.Error(codes.FailedPrecondition, "booking is not confirmed by platform")
	}

	review := &domain.Review{
		UserID:      in.UserID,
		UserName:    in.UserName,
		VenueID:     in.VenueID,
		MasterID:    in.MasterID,
		Rating:      in.Rating,
		Text:        in.Text,
		IsVerified:  isVerified,
		IsAnonymous: in.IsAnonymous,
	}

	if err := uc.repo.Create(ctx, review); err != nil {
		return nil, fmt.Errorf("create review: %w", err)
	}

	if hasVenue {
		if err := uc.repo.UpdateVenueRating(ctx, in.VenueID); err != nil {
			return nil, fmt.Errorf("update venue rating: %w", err)
		}

		_ = uc.publisher.PublishReviewCreated(events.ReviewCreatedEvent{
			ReviewID: review.ID,
			UserID:   review.UserID,
			VenueID:  review.VenueID,
			Rating:   review.Rating,
		})

		vr, err := uc.repo.GetVenueRating(ctx, in.VenueID)
		if err == nil {
			_, _ = uc.venueClient.UpdateRating(ctx, &venuev1.UpdateRatingRequest{
				VenueId:     in.VenueID,
				AvgRating:   vr.AvgRating,
				ReviewCount: vr.ReviewCount,
			})
		}
	}

	return review, nil
}

func (uc *ReviewUseCase) GetReview(ctx context.Context, id string) (*domain.Review, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *ReviewUseCase) ListVenueReviews(ctx context.Context, venueID string, page, pageSize int32) ([]*domain.Review, int32, error) {
	return uc.repo.ListByVenue(ctx, venueID, page, pageSize)
}

func (uc *ReviewUseCase) ListMasterReviews(ctx context.Context, masterID string, page, pageSize int32) ([]*domain.Review, int32, error) {
	return uc.repo.ListByMaster(ctx, masterID, page, pageSize)
}

func (uc *ReviewUseCase) GetVenueRating(ctx context.Context, venueID string) (*domain.VenueRating, error) {
	return uc.repo.GetVenueRating(ctx, venueID)
}
