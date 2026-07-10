package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
	"github.com/tienlao/agregator/services/review-service/internal/events"
	"github.com/tienlao/agregator/services/review-service/internal/kpi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ReviewUseCase struct {
	repo          domain.ReviewRepository
	outboxRepo    domain.OutboxRepository
	bookingClient bookingv1.BookingServiceClient
	masterClient  masterv1.MasterServiceClient
}

// NewReviewUseCaseWithOutbox constructs a ReviewUseCase.
// outboxRepo is required for transactional outbox writes; pass a real OutboxRepository.
func NewReviewUseCaseWithOutbox(
	repo domain.ReviewRepository,
	outboxRepo domain.OutboxRepository,
	bookingClient bookingv1.BookingServiceClient,
	masterClient masterv1.MasterServiceClient,
) *ReviewUseCase {
	return &ReviewUseCase{
		repo:          repo,
		outboxRepo:    outboxRepo,
		bookingClient: bookingClient,
		masterClient:  masterClient,
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
			// Preserve the upstream gRPC status (e.g. Unavailable, NotFound).
			return nil, err
		}
		isVerified = hasVisited.GetHasCompleted()
	} else if hasMaster {
		if uc.masterClient != nil {
			hasVisited, err := uc.masterClient.HasCompletedMasterBooking(ctx, &masterv1.HasCompletedMasterBookingRequest{
				ClientUserId: in.UserID,
				MasterId:     in.MasterID,
			})
			if err != nil {
				// Preserve the upstream gRPC status.
				return nil, err
			}
			isVerified = hasVisited.GetHasCompleted()
		}
	}
	if !isVerified {
		return nil, pkgerr.FailedPrecondition("booking is not confirmed by platform")
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

	// --- Atomic write: review + outbox entry in one transaction ---
	tx, err := uc.repo.BeginTx(ctx)
	if err != nil {
		log.Error().Err(err).Msg("CreateReview: begin tx failed")
		return nil, pkgerr.Internal("failed to begin transaction")
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = uc.repo.CreateTx(ctx, tx, review); err != nil {
		// AlreadyExists is already a gRPC status from the repository layer;
		// other errors are DB-internal and must not leak details to the client.
		if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
			return nil, err
		}
		log.Error().Err(err).Msg("CreateReview: persist review failed")
		return nil, pkgerr.Internal("failed to persist review")
	}

	if uc.outboxRepo != nil {
		targetType := events.TargetTypeVenue
		if hasMaster {
			targetType = events.TargetTypeMaster
		}
		payload, merr := json.Marshal(events.ReviewCreatedEvent{
			ReviewID:   review.ID,
			UserID:     review.UserID,
			TargetType: targetType,
			VenueID:    review.VenueID,
			MasterID:   review.MasterID,
			Rating:     review.Rating,
		})
		if merr != nil {
			// json.Marshal of a known struct should never fail; log the underlying
			// error so the cause is visible in service logs even though the client
			// only receives a generic Internal status.
			log.Error().Err(merr).Str("review_id", review.ID).
				Msg("outbox: failed to marshal ReviewCreatedEvent payload")
			err = pkgerr.Internal("failed to marshal outbox payload")
			return nil, err
		}
		outboxEntry := &domain.OutboxEntry{
			AggregateID: review.ID,
			EventType:   events.SubjectReviewCreated,
			Payload:     payload,
		}
		if merr = uc.outboxRepo.CreateTx(ctx, tx, outboxEntry); merr != nil {
			log.Error().Err(merr).Str("review_id", review.ID).Msg("CreateReview: write outbox entry failed")
			err = pkgerr.Internal("failed to write outbox entry")
			return nil, err
		}
	}

	// Update the ratings cache inside the same transaction so that the cache
	// increment is atomic with the review insert and outbox write.
	// If either fails the whole transaction rolls back — no split-brain window.
	if hasVenue {
		if err = uc.repo.UpdateVenueRatingTx(ctx, tx, in.VenueID, in.Rating); err != nil {
			log.Error().Err(err).Str("venue_id", in.VenueID).Msg("CreateReview: update venue rating cache failed")
			err = pkgerr.Internal("failed to update venue rating cache")
			return nil, err
		}
	}
	if hasMaster {
		if err = uc.repo.UpdateMasterRatingTx(ctx, tx, in.MasterID, in.Rating); err != nil {
			log.Error().Err(err).Str("master_id", in.MasterID).Msg("CreateReview: update master rating cache failed")
			err = pkgerr.Internal("failed to update master rating cache")
			return nil, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("CreateReview: commit tx failed")
		return nil, pkgerr.Internal("failed to commit transaction")
	}

	kpi.Review(in.Rating)
	return review, nil
}

func (uc *ReviewUseCase) GetReview(ctx context.Context, id string) (*domain.Review, error) {
	return uc.repo.GetByID(ctx, id)
}

const maxPage = 1000

func validatePagination(page, pageSize int32) error {
	if page < 1 || page > maxPage {
		return pkgerr.InvalidArgument("page must be in [1, 1000]")
	}
	if pageSize < 1 || pageSize > 50 {
		return pkgerr.InvalidArgument("page_size must be in [1, 50]")
	}
	return nil
}

func (uc *ReviewUseCase) ListVenueReviews(ctx context.Context, venueID string, onlyUnanswered bool, page, pageSize int32) ([]*domain.Review, int32, error) {
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return uc.repo.ListByVenue(ctx, venueID, onlyUnanswered, page, pageSize)
}

// maxReplyLen bounds an owner reply so a single response can't bloat the row or
// the public venue card.
const maxReplyLen = 2000

// ReplyToReview creates or replaces the venue owner's reply to a review. Venue
// ownership is verified upstream (api-gateway); venue_id scopes the write so the
// reply can only attach to a review that belongs to that venue.
func (uc *ReviewUseCase) ReplyToReview(ctx context.Context, reviewID, venueID, authorUserID, body string) (*domain.ReviewReply, error) {
	if reviewID == "" || venueID == "" {
		return nil, pkgerr.InvalidArgument("review_id and venue_id are required")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, pkgerr.InvalidArgument("reply text is required")
	}
	if utf8.RuneCountInString(body) > maxReplyLen {
		return nil, pkgerr.InvalidArgument("reply text is too long")
	}
	return uc.repo.UpsertReply(ctx, reviewID, venueID, authorUserID, body)
}

// DeleteReviewReply removes the owner reply, scoped to venueID.
func (uc *ReviewUseCase) DeleteReviewReply(ctx context.Context, reviewID, venueID string) error {
	if reviewID == "" || venueID == "" {
		return pkgerr.InvalidArgument("review_id and venue_id are required")
	}
	return uc.repo.DeleteReply(ctx, reviewID, venueID)
}

func (uc *ReviewUseCase) GetVenueReviewSummary(ctx context.Context, venueID string) (*domain.VenueReviewSummary, error) {
	if venueID == "" {
		return nil, pkgerr.InvalidArgument("venue_id is required")
	}
	return uc.repo.GetVenueReviewSummary(ctx, venueID)
}

func (uc *ReviewUseCase) ListMasterReviews(ctx context.Context, masterID string, page, pageSize int32) ([]*domain.Review, int32, error) {
	if err := validatePagination(page, pageSize); err != nil {
		return nil, 0, err
	}
	return uc.repo.ListByMaster(ctx, masterID, page, pageSize)
}

func (uc *ReviewUseCase) GetVenueRating(ctx context.Context, venueID string) (*domain.VenueRating, error) {
	return uc.repo.GetVenueRating(ctx, venueID)
}

func (uc *ReviewUseCase) GetMasterRating(ctx context.Context, masterID string) (*domain.MasterRating, error) {
	return uc.repo.GetMasterRating(ctx, masterID)
}
