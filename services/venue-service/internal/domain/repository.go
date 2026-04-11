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
	City      string
	Cities    []string
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
	ReplaceVenueServices(ctx context.Context, venueID uuid.UUID, services []VenueService) error
	GetByID(ctx context.Context, id uuid.UUID) (*Venue, error)
	GetBySlug(ctx context.Context, slug string) (*Venue, error)
	List(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*ListResult, error)
	Search(ctx context.Context, params SearchParams) (*ListResult, error)
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Venue, error)
	ListByStatus(ctx context.Context, status string, page, pageSize int32) (*ListResult, error)
	UpdateStatus(ctx context.Context, venueID uuid.UUID, status, comment string, moderatedBy uuid.UUID) error
	ResetToPendingReview(ctx context.Context, venueID uuid.UUID) error
	// SubmitDraftForReview sets status pending_review for venue in draft owned by ownerID. Returns false if no row updated.
	SubmitDraftForReview(ctx context.Context, venueID, ownerID uuid.UUID) (updated bool, err error)
	InsertModerationHistory(ctx context.Context, entry *ModerationHistoryEntry) error
	GetModerationHistory(ctx context.Context, venueID uuid.UUID) ([]ModerationHistoryEntry, error)
	UpdateRating(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error
	CheckSlot(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo string) (bool, error)
	ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string) error
	ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error
	CreateManualSlotBlock(ctx context.Context, venueID uuid.UUID, date, timeFrom, timeTo, note string) (uuid.UUID, error)
	DeleteManualSlotBlock(ctx context.Context, venueID, blockID uuid.UUID) (deleted bool, err error)
	ListManualSlotBlocks(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]ManualSlotBlock, error)

	AddVenuePhoto(ctx context.Context, venueID uuid.UUID, url string) (*VenuePhoto, error)
	DeleteVenuePhoto(ctx context.Context, venueID, photoID uuid.UUID) (deletedURL string, err error)
	SetVenueCoverPhoto(ctx context.Context, venueID, photoID uuid.UUID) error

	ReplaceVenueHalls(ctx context.Context, venueID uuid.UUID, items []VenueHallUpsert) error
	AddVenueHallPhoto(ctx context.Context, venueID, hallID uuid.UUID, url string) (*VenueHallPhoto, error)
	DeleteVenueHallPhoto(ctx context.Context, venueID, hallID, photoID uuid.UUID) (deletedURL string, err error)
	SetVenueHallCoverPhoto(ctx context.Context, venueID, hallID, photoID uuid.UUID) error
}
