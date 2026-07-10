package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
	"github.com/tienlao/agregator/services/review-service/internal/events"
)

// newUC is a convenience constructor used by most tests.
// It wires a mockOutboxRepo so outbox writes are captured but never hit Postgres.
func newUC(repo *mockReviewRepo, outbox *mockOutboxRepo, booking *mockBookingClient) *ReviewUseCase {
	return NewReviewUseCaseWithOutbox(repo, outbox, booking, nil)
}

// confirmedBooking returns a booking client that always says "completed".
func confirmedBooking() *mockBookingClient {
	return &mockBookingClient{
		HasCompletedBookingFunc: func(_ context.Context, _ *bookingv1.HasCompletedBookingRequest, _ ...grpc.CallOption) (*bookingv1.HasCompletedBookingResponse, error) {
			return &bookingv1.HasCompletedBookingResponse{HasCompleted: true}, nil
		},
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestCreateReview_InvalidRating(t *testing.T) {
	ctx := context.Background()
	uc := newUC(&mockReviewRepo{}, &mockOutboxRepo{}, &mockBookingClient{})

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

func TestCreateReview_InvalidTargetCombination(t *testing.T) {
	ctx := context.Background()
	uc := newUC(&mockReviewRepo{}, &mockOutboxRepo{}, &mockBookingClient{})

	// both empty
	_, err := uc.CreateReview(ctx, CreateReviewInput{UserID: "u1", Rating: 5, Text: "ok"})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())

	// both set
	_, err = uc.CreateReview(ctx, CreateReviewInput{UserID: "u1", Rating: 5, Text: "ok", VenueID: "v1", MasterID: "m1"})
	require.Error(t, err)
	st, ok = status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestCreateReview_UnconfirmedVenueBooking(t *testing.T) {
	ctx := context.Background()
	uc := newUC(&mockReviewRepo{}, &mockOutboxRepo{}, &mockBookingClient{
		HasCompletedBookingFunc: func(_ context.Context, _ *bookingv1.HasCompletedBookingRequest, _ ...grpc.CallOption) (*bookingv1.HasCompletedBookingResponse, error) {
			return &bookingv1.HasCompletedBookingResponse{HasCompleted: false}, nil
		},
	})
	got, err := uc.CreateReview(ctx, CreateReviewInput{UserID: "u1", VenueID: "v1", Rating: 5, Text: "ok"})
	require.Error(t, err)
	require.Nil(t, got)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
}

// ---------------------------------------------------------------------------
// Success path: review + outbox written atomically
// ---------------------------------------------------------------------------

func TestCreateReview_Success_AtomicOutbox(t *testing.T) {
	ctx := context.Background()

	var createdReview *domain.Review
	var outboxEntry *domain.OutboxEntry
	type ratingUpdate struct {
		venueID string
		rating  int32
	}
	var ratingUpdates []ratingUpdate

	repo := &mockReviewRepo{
		CreateTxFunc: func(ctx context.Context, tx pgx.Tx, review *domain.Review) error {
			review.ID = "review-id-1"
			createdReview = review
			return nil
		},
		UpdateVenueRatingTxFunc: func(ctx context.Context, tx pgx.Tx, venueID string, rating int32) error {
			ratingUpdates = append(ratingUpdates, ratingUpdate{venueID, rating})
			return nil
		},
	}
	outboxRepo := &mockOutboxRepo{
		CreateTxFunc: func(ctx context.Context, tx pgx.Tx, entry *domain.OutboxEntry) error {
			outboxEntry = entry
			return nil
		},
	}

	got, err := newUC(repo, outboxRepo, confirmedBooking()).CreateReview(ctx, CreateReviewInput{
		UserID:  "user-1",
		VenueID: "venue-1",
		Rating:  4,
		Text:    "great",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "review-id-1", got.ID)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "venue-1", got.VenueID)
	assert.Equal(t, int32(4), got.Rating)
	assert.True(t, got.IsVerified)

	// Review persisted
	require.NotNil(t, createdReview)

	// Outbox entry created in the same transaction
	require.NotNil(t, outboxEntry, "outbox entry must be written for venue reviews")
	assert.Equal(t, events.SubjectReviewCreated, outboxEntry.EventType)
	assert.Equal(t, "review-id-1", outboxEntry.AggregateID)

	// Payload is valid JSON with the right fields
	var evt events.ReviewCreatedEvent
	require.NoError(t, json.Unmarshal(outboxEntry.Payload, &evt))
	assert.Equal(t, "review-id-1", evt.ReviewID)
	assert.Equal(t, "user-1", evt.UserID)
	assert.Equal(t, "venue-1", evt.VenueID)
	assert.Equal(t, int32(4), evt.Rating)
	assert.Equal(t, events.TargetTypeVenue, evt.TargetType)

	// Transaction committed
	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.committed, "transaction must be committed on success")
	assert.False(t, repo.tx.rolledBack)

	// Rating cache updated (post-commit, best-effort) with the correct rating value.
	require.Len(t, ratingUpdates, 1)
	assert.Equal(t, "venue-1", ratingUpdates[0].venueID)
	assert.Equal(t, int32(4), ratingUpdates[0].rating, "rating forwarded to incremental cache update")
}

// ---------------------------------------------------------------------------
// Idempotency: AlreadyExists from the repository reaches the caller unchanged.
// This pins the passthrough in usecase so that wrapping the error (e.g. with
// fmt.Errorf) is caught immediately as a regression.
// ---------------------------------------------------------------------------

func TestCreateReview_AlreadyExists_PassedThrough(t *testing.T) {
	ctx := context.Background()

	repo := &mockReviewRepo{
		CreateTxFunc: func(_ context.Context, _ pgx.Tx, _ *domain.Review) error {
			// Simulate the unique-constraint mapping done by the repository layer.
			return pkgerr.AlreadyExists("review already exists for this target")
		},
	}

	_, err := newUC(repo, &mockOutboxRepo{}, confirmedBooking()).CreateReview(ctx, CreateReviewInput{
		UserID:  "user-1",
		VenueID: "venue-1",
		Rating:  5,
		Text:    "duplicate",
	})

	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "error must be a gRPC status; got %T: %v", err, err)
	assert.Equal(t, codes.AlreadyExists, st.Code(),
		"AlreadyExists must not be swallowed or replaced by Internal/Unknown")

	// Transaction must have been rolled back (no commit after the failed insert).
	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.rolledBack)
	assert.False(t, repo.tx.committed)
}

// ---------------------------------------------------------------------------
// Atomicity: repo error rolls back the transaction; outbox entry not written
// ---------------------------------------------------------------------------

func TestCreateReview_RepoError_RollsBackTx(t *testing.T) {
	ctx := context.Background()

	repo := &mockReviewRepo{
		CreateTxFunc: func(ctx context.Context, tx pgx.Tx, review *domain.Review) error {
			return fmt.Errorf("db: connection reset")
		},
	}
	outboxRepo := &mockOutboxRepo{}

	got, err := newUC(repo, outboxRepo, confirmedBooking()).CreateReview(ctx, CreateReviewInput{
		UserID:  "user-1",
		VenueID: "venue-1",
		Rating:  3,
		Text:    "meh",
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, outboxRepo.entries, "outbox must not be written when review insert fails")

	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.rolledBack, "tx must be rolled back on repo error")
	assert.False(t, repo.tx.committed)
}

// ---------------------------------------------------------------------------
// Atomicity: outbox error rolls back the transaction
// ---------------------------------------------------------------------------

func TestCreateReview_OutboxError_RollsBackTx(t *testing.T) {
	ctx := context.Background()

	repo := &mockReviewRepo{
		CreateTxFunc: func(ctx context.Context, tx pgx.Tx, review *domain.Review) error {
			review.ID = "r-1"
			return nil
		},
	}
	outboxRepo := &mockOutboxRepo{
		CreateTxFunc: func(ctx context.Context, tx pgx.Tx, entry *domain.OutboxEntry) error {
			return fmt.Errorf("outbox insert failed")
		},
	}

	got, err := newUC(repo, outboxRepo, confirmedBooking()).CreateReview(ctx, CreateReviewInput{
		UserID:  "user-1",
		VenueID: "venue-1",
		Rating:  5,
		Text:    "perfect but outbox broke",
	})

	require.Error(t, err)
	assert.Nil(t, got)

	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.rolledBack, "tx must roll back when outbox insert fails")
	assert.False(t, repo.tx.committed)
}

// ---------------------------------------------------------------------------
// Atomicity: rating cache error inside the transaction rolls it back entirely.
// This is the key invariant of the refactor: the cache update now happens
// before tx.Commit, so a cache failure prevents the review from being
// committed — no split-brain between reviews table and ratings_cache.
// ---------------------------------------------------------------------------

func TestCreateReview_RatingCacheError_RollsBackTx(t *testing.T) {
	ctx := context.Background()

	repo := &mockReviewRepo{
		CreateTxFunc: func(_ context.Context, _ pgx.Tx, review *domain.Review) error {
			review.ID = "r-cache-fail"
			return nil
		},
		UpdateVenueRatingTxFunc: func(_ context.Context, _ pgx.Tx, _ string, _ int32) error {
			return fmt.Errorf("db: deadlock detected")
		},
	}
	outboxRepo := &mockOutboxRepo{}

	got, err := newUC(repo, outboxRepo, confirmedBooking()).CreateReview(ctx, CreateReviewInput{
		UserID:  "user-1",
		VenueID: "venue-1",
		Rating:  5,
		Text:    "cache will fail",
	})

	require.Error(t, err)
	assert.Nil(t, got)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())

	// Transaction must be rolled back — the review insert is undone together
	// with the failed cache update, so ratings_cache stays consistent.
	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.rolledBack, "tx must roll back when cache update fails inside the transaction")
	assert.False(t, repo.tx.committed, "tx must NOT commit when cache update fails")
}

// confirmedMasterBooking returns a master client that always says "completed".
func confirmedMasterBooking() *mockMasterClient {
	return &mockMasterClient{hasCompleted: true}
}

// ---------------------------------------------------------------------------
// Master target: success — outbox entry written, master rating updated,
//                          no venue ops touched.
// ---------------------------------------------------------------------------

func TestCreateReview_MasterTarget_Success(t *testing.T) {
	ctx := context.Background()

	var createdReview *domain.Review
	var outboxEntry *domain.OutboxEntry
	var venueRatingUpdates int
	type masterUpdate struct {
		masterID string
		rating   int32
	}
	var masterRatingUpdates []masterUpdate

	repo := &mockReviewRepo{
		CreateTxFunc: func(_ context.Context, _ pgx.Tx, review *domain.Review) error {
			review.ID = "r-master-1"
			createdReview = review
			return nil
		},
		UpdateVenueRatingTxFunc: func(_ context.Context, _ pgx.Tx, _ string, _ int32) error {
			venueRatingUpdates++
			return nil
		},
		UpdateMasterRatingTxFunc: func(_ context.Context, _ pgx.Tx, masterID string, rating int32) error {
			masterRatingUpdates = append(masterRatingUpdates, masterUpdate{masterID, rating})
			return nil
		},
	}
	outboxRepo := &mockOutboxRepo{
		CreateTxFunc: func(_ context.Context, _ pgx.Tx, entry *domain.OutboxEntry) error {
			outboxEntry = entry
			return nil
		},
	}

	uc := NewReviewUseCaseWithOutbox(repo, outboxRepo, &mockBookingClient{}, confirmedMasterBooking())
	got, err := uc.CreateReview(ctx, CreateReviewInput{
		UserID:   "user-1",
		MasterID: "master-1",
		Rating:   5,
		Text:     "excellent",
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "r-master-1", got.ID)
	assert.Equal(t, "master-1", got.MasterID)
	assert.Equal(t, "", got.VenueID)
	assert.True(t, got.IsVerified)

	// Review persisted
	require.NotNil(t, createdReview)

	// Outbox entry written with MasterID populated
	require.NotNil(t, outboxEntry, "outbox entry must be written for master reviews")
	assert.Equal(t, events.SubjectReviewCreated, outboxEntry.EventType)
	assert.Equal(t, "r-master-1", outboxEntry.AggregateID)

	var evt events.ReviewCreatedEvent
	require.NoError(t, json.Unmarshal(outboxEntry.Payload, &evt))
	assert.Equal(t, "r-master-1", evt.ReviewID)
	assert.Equal(t, "master-1", evt.MasterID)
	assert.Equal(t, "", evt.VenueID)
	assert.Equal(t, int32(5), evt.Rating)
	assert.Equal(t, events.TargetTypeMaster, evt.TargetType)

	// Master rating cache updated with the correct rating value (incremental O(1) path).
	require.Len(t, masterRatingUpdates, 1)
	assert.Equal(t, "master-1", masterRatingUpdates[0].masterID)
	assert.Equal(t, int32(5), masterRatingUpdates[0].rating, "rating forwarded to incremental cache update")

	// Venue rating NOT touched
	assert.Equal(t, 0, venueRatingUpdates, "venue rating must not be updated for master reviews")

	// Transaction committed
	require.NotNil(t, repo.tx)
	assert.True(t, repo.tx.committed)
}

// ---------------------------------------------------------------------------
// Master target: no masterClient → FailedPrecondition, no side effects
// ---------------------------------------------------------------------------

func TestCreateReview_MasterTarget_NoMasterClient_FailedPrecondition(t *testing.T) {
	ctx := context.Background()

	var venueRatingUpdates, masterRatingUpdates int
	repo := &mockReviewRepo{
		UpdateVenueRatingTxFunc:  func(_ context.Context, _ pgx.Tx, _ string, _ int32) error { venueRatingUpdates++; return nil },
		UpdateMasterRatingTxFunc: func(_ context.Context, _ pgx.Tx, _ string, _ int32) error { masterRatingUpdates++; return nil },
	}
	outboxRepo := &mockOutboxRepo{}

	// masterClient nil → guard skips HasCompletedMasterBooking → isVerified=false
	uc := NewReviewUseCaseWithOutbox(repo, outboxRepo, &mockBookingClient{}, nil)
	got, err := uc.CreateReview(ctx, CreateReviewInput{
		UserID:   "user-1",
		MasterID: "master-1",
		Rating:   5,
		Text:     "solid session",
	})

	require.Error(t, err)
	require.Nil(t, got)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
	assert.Equal(t, 0, venueRatingUpdates)
	assert.Equal(t, 0, masterRatingUpdates)
	assert.Empty(t, outboxRepo.entries)
}

// ---------------------------------------------------------------------------
// Venue event payload carries VenueID, not MasterID
// ---------------------------------------------------------------------------

func TestCreateReview_VenueTarget_EventPayload(t *testing.T) {
	ctx := context.Background()

	var outboxEntry *domain.OutboxEntry
	repo := &mockReviewRepo{
		CreateTxFunc: func(_ context.Context, _ pgx.Tx, review *domain.Review) error {
			review.ID = "r-venue-1"
			return nil
		},
	}
	outboxRepo := &mockOutboxRepo{
		CreateTxFunc: func(_ context.Context, _ pgx.Tx, entry *domain.OutboxEntry) error {
			outboxEntry = entry
			return nil
		},
	}

	_, err := newUC(repo, outboxRepo, confirmedBooking()).CreateReview(ctx, CreateReviewInput{
		UserID:  "user-1",
		VenueID: "venue-1",
		Rating:  3,
		Text:    "ok",
	})
	require.NoError(t, err)

	require.NotNil(t, outboxEntry)
	var evt events.ReviewCreatedEvent
	require.NoError(t, json.Unmarshal(outboxEntry.Payload, &evt))
	assert.Equal(t, "venue-1", evt.VenueID)
	assert.Equal(t, "", evt.MasterID, "MasterID must be empty for venue reviews")
	assert.Equal(t, events.TargetTypeVenue, evt.TargetType)
}

// ---------------------------------------------------------------------------
// Pagination validation
// ---------------------------------------------------------------------------

func TestListVenueReviews_InvalidPagination(t *testing.T) {
	ctx := context.Background()
	uc := NewReviewUseCaseWithOutbox(&mockReviewRepo{}, &mockOutboxRepo{}, &mockBookingClient{}, nil)

	cases := []struct {
		name     string
		page     int32
		pageSize int32
	}{
		{"page zero", 0, 10},
		{"page negative", -1, 10},
		{"page over cap", 1001, 10},
		{"pageSize zero", 1, 0},
		{"pageSize negative", 1, -1},
		{"pageSize over max", 1, 51},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := uc.ListVenueReviews(ctx, "v1", false, tc.page, tc.pageSize)
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code(), "expected InvalidArgument for page=%d pageSize=%d", tc.page, tc.pageSize)
		})
	}
}

func TestListMasterReviews_InvalidPagination(t *testing.T) {
	ctx := context.Background()
	uc := NewReviewUseCaseWithOutbox(&mockReviewRepo{}, &mockOutboxRepo{}, &mockBookingClient{}, nil)

	cases := []struct {
		name     string
		page     int32
		pageSize int32
	}{
		{"page zero", 0, 10},
		{"pageSize zero", 1, 0},
		{"pageSize over max", 1, 51},
		{"page over cap", 1001, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := uc.ListMasterReviews(ctx, "m1", tc.page, tc.pageSize)
			require.Error(t, err)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.InvalidArgument, st.Code())
		})
	}
}

// ---------------------------------------------------------------------------
// Read operations
// ---------------------------------------------------------------------------

func TestListVenueReviews(t *testing.T) {
	ctx := context.Background()
	want := []*domain.Review{{ID: "r1", VenueID: "v1"}}
	repo := &mockReviewRepo{
		ListByVenueFunc: func(_ context.Context, venueID string, onlyUnanswered bool, page, pageSize int32) ([]*domain.Review, int32, error) {
			assert.Equal(t, "v1", venueID)
			assert.False(t, onlyUnanswered)
			assert.Equal(t, int32(2), page)
			assert.Equal(t, int32(10), pageSize)
			return want, 99, nil
		},
	}
	uc := NewReviewUseCaseWithOutbox(repo, &mockOutboxRepo{}, &mockBookingClient{}, nil)

	got, total, err := uc.ListVenueReviews(ctx, "v1", false, 2, 10)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, int32(99), total)
}

func TestReplyToReview_Validation(t *testing.T) {
	ctx := context.Background()
	uc := NewReviewUseCaseWithOutbox(&mockReviewRepo{}, &mockOutboxRepo{}, &mockBookingClient{}, nil)

	cases := []struct {
		name              string
		reviewID, venueID string
		body              string
	}{
		{"missing review_id", "", "v1", "hi"},
		{"missing venue_id", "r1", "", "hi"},
		{"empty body", "r1", "v1", "   "},
		{"body too long", "r1", "v1", strings.Repeat("я", maxReplyLen+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uc.ReplyToReview(ctx, tc.reviewID, tc.venueID, "owner1", tc.body)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestReplyToReview_TrimsAndPassesThrough(t *testing.T) {
	ctx := context.Background()
	repo := &mockReviewRepo{
		UpsertReplyFunc: func(_ context.Context, reviewID, venueID, author, body string) (*domain.ReviewReply, error) {
			assert.Equal(t, "r1", reviewID)
			assert.Equal(t, "v1", venueID)
			assert.Equal(t, "owner1", author)
			assert.Equal(t, "спасибо", body) // trimmed
			return &domain.ReviewReply{Body: body, AuthorUserID: author}, nil
		},
	}
	uc := NewReviewUseCaseWithOutbox(repo, &mockOutboxRepo{}, &mockBookingClient{}, nil)

	got, err := uc.ReplyToReview(ctx, "r1", "v1", "owner1", "  спасибо  ")
	require.NoError(t, err)
	assert.Equal(t, "спасибо", got.Body)
}

func TestDeleteReviewReply_Validation(t *testing.T) {
	ctx := context.Background()
	uc := NewReviewUseCaseWithOutbox(&mockReviewRepo{}, &mockOutboxRepo{}, &mockBookingClient{}, nil)
	err := uc.DeleteReviewReply(ctx, "", "v1")
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetVenueReviewSummary(t *testing.T) {
	ctx := context.Background()
	want := &domain.VenueReviewSummary{VenueID: "v1", AvgRating: 4.6, ReviewCount: 128, UnansweredCount: 14}
	repo := &mockReviewRepo{
		GetVenueReviewSummaryFunc: func(_ context.Context, venueID string) (*domain.VenueReviewSummary, error) {
			assert.Equal(t, "v1", venueID)
			return want, nil
		},
	}
	uc := NewReviewUseCaseWithOutbox(repo, &mockOutboxRepo{}, &mockBookingClient{}, nil)

	got, err := uc.GetVenueReviewSummary(ctx, "v1")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	_, err = uc.GetVenueReviewSummary(ctx, "")
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestGetReview(t *testing.T) {
	ctx := context.Background()
	want := &domain.Review{ID: "r1", Text: "ok"}
	repo := &mockReviewRepo{
		GetByIDFunc: func(_ context.Context, id string) (*domain.Review, error) {
			assert.Equal(t, "r1", id)
			return want, nil
		},
	}
	uc := NewReviewUseCaseWithOutbox(repo, &mockOutboxRepo{}, &mockBookingClient{}, nil)

	got, err := uc.GetReview(ctx, "r1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGetVenueRating(t *testing.T) {
	ctx := context.Background()
	want := &domain.VenueRating{VenueID: "v1", AvgRating: 4.5, ReviewCount: 3, UpdatedAt: time.Unix(1, 0).UTC()}
	repo := &mockReviewRepo{
		GetVenueRatingFunc: func(_ context.Context, venueID string) (*domain.VenueRating, error) {
			assert.Equal(t, "v1", venueID)
			return want, nil
		},
	}
	uc := NewReviewUseCaseWithOutbox(repo, &mockOutboxRepo{}, &mockBookingClient{}, nil)

	got, err := uc.GetVenueRating(ctx, "v1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
