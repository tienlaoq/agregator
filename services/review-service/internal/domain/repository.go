package domain

import "context"

type ReviewRepository interface {
	Create(ctx context.Context, review *Review) error
	GetByID(ctx context.Context, id string) (*Review, error)
	ListByVenue(ctx context.Context, venueID string, page, pageSize int32) ([]*Review, int32, error)
	GetVenueRating(ctx context.Context, venueID string) (*VenueRating, error)
	UpdateVenueRating(ctx context.Context, venueID string) error
}
