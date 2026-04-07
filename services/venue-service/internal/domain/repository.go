package domain

import (
	"context"

	"github.com/google/uuid"
)

type ListResult struct {
	Venues   []Venue
	Total    int32
	Page     int32
	PageSize int32
}

type SearchParams struct {
	Query     string
	Lat       float64
	Lng       float64
	RadiusKM  float64
	VenueType string
	PriceMin  int64
	PriceMax  int64
	RatingMin float64
	Amenities []string
	Page      int32
	PageSize  int32
}

type VenueRepository interface {
	Create(ctx context.Context, venue *Venue) error
	Update(ctx context.Context, venue *Venue) error
	GetByID(ctx context.Context, id uuid.UUID) (*Venue, error)
	GetBySlug(ctx context.Context, slug string) (*Venue, error)
	List(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*ListResult, error)
	Search(ctx context.Context, params SearchParams) (*ListResult, error)
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Venue, error)
	UpdateRating(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error
	CheckSlot(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo string) (bool, error)
	ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string) error
	ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error
}
