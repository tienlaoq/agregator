package domain

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type ReviewRepository interface {
	// BeginTx starts a new transaction on the underlying pool.
	// The usecase uses it to wrap review + outbox inserts atomically.
	BeginTx(ctx context.Context) (pgx.Tx, error)

	// CreateTx inserts a review within the provided transaction.
	CreateTx(ctx context.Context, tx pgx.Tx, review *Review) error

	GetByID(ctx context.Context, id string) (*Review, error)
	// ListByVenue returns a page of venue reviews, each carrying its owner reply
	// (Reply) when one exists. When onlyUnanswered is true, only reviews without
	// a reply are returned — the owner-triage view.
	ListByVenue(ctx context.Context, venueID string, onlyUnanswered bool, page, pageSize int32) ([]*Review, int32, error)
	ListByMaster(ctx context.Context, masterID string, page, pageSize int32) ([]*Review, int32, error)
	GetVenueRating(ctx context.Context, venueID string) (*VenueRating, error)
	// GetVenueReviewSummary returns the cached aggregate plus the count of
	// reviews still awaiting an owner reply, for the owner dashboard header.
	GetVenueReviewSummary(ctx context.Context, venueID string) (*VenueReviewSummary, error)

	// UpsertReply creates or replaces the owner reply for reviewID. It verifies
	// the review belongs to venueID and returns NotFound otherwise, so an owner
	// cannot reply to another venue's review by guessing an id.
	UpsertReply(ctx context.Context, reviewID, venueID, authorUserID, body string) (*ReviewReply, error)
	// DeleteReply removes the owner reply for reviewID scoped to venueID.
	// Returns NotFound when there is no reply to delete.
	DeleteReply(ctx context.Context, reviewID, venueID string) error
	// UpdateVenueRatingTx increments the cached rating for venueID within tx in O(1).
	// Must be called before tx.Commit so the cache update is atomic with the review insert.
	UpdateVenueRatingTx(ctx context.Context, tx pgx.Tx, venueID string, rating int32) error

	GetMasterRating(ctx context.Context, masterID string) (*MasterRating, error)
	// UpdateMasterRatingTx increments the cached rating for masterID within tx in O(1).
	// Must be called before tx.Commit so the cache update is atomic with the review insert.
	UpdateMasterRatingTx(ctx context.Context, tx pgx.Tx, masterID string, rating int32) error
}
