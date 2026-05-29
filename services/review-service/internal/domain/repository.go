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
	ListByVenue(ctx context.Context, venueID string, page, pageSize int32) ([]*Review, int32, error)
	ListByMaster(ctx context.Context, masterID string, page, pageSize int32) ([]*Review, int32, error)
	GetVenueRating(ctx context.Context, venueID string) (*VenueRating, error)
	// UpdateVenueRatingTx increments the cached rating for venueID within tx in O(1).
	// Must be called before tx.Commit so the cache update is atomic with the review insert.
	UpdateVenueRatingTx(ctx context.Context, tx pgx.Tx, venueID string, rating int32) error

	GetMasterRating(ctx context.Context, masterID string) (*MasterRating, error)
	// UpdateMasterRatingTx increments the cached rating for masterID within tx in O(1).
	// Must be called before tx.Commit so the cache update is atomic with the review insert.
	UpdateMasterRatingTx(ctx context.Context, tx pgx.Tx, masterID string, rating int32) error
}
