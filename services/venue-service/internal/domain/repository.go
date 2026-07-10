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

// CityCount — город и число активных заведений в нём (агрегат для «Популярных»).
type CityCount struct {
	City  string
	Count int32
}

type SearchParams struct {
	Query     string
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
	ReplaceVenueServices(ctx context.Context, venueID, ownerID uuid.UUID, services []VenueService) error
	GetByID(ctx context.Context, id uuid.UUID) (*Venue, error)
	// GetByIDs fetches multiple venues in a single query. Unknown ids are omitted.
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]Venue, error)
	GetBySlug(ctx context.Context, slug string) (*Venue, error)
	List(ctx context.Context, page, pageSize int32, venueType, sortBy string) (*ListResult, error)
	Search(ctx context.Context, params SearchParams) (*ListResult, error)
	// PopularCities returns active-venue counts per city, most first, capped at limit.
	PopularCities(ctx context.Context, limit int32) ([]CityCount, error)
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Venue, error)

	// SuspendByOwner sets every not-yet-suspended venue owned by ownerID to
	// status "suspended" (is_active=false) and returns the number of rows
	// transitioned. Used by the account-deletion cascade so a deleted owner's
	// venues drop out of public listings.
	SuspendByOwner(ctx context.Context, ownerID uuid.UUID) (int64, error)

	// IsVenueMember returns true when userID is the venue owner or a row in
	// venue_staff. Legacy strangler-fig bridge: crm-service owns the source of
	// truth, but venue-service still reads the shared `venue_staff` table
	// directly in phase B so manual slot blocks and other owner-only
	// operations stay local. When CRM gets its own database, replace this with
	// a gRPC call to crm.GetManagementAccess.
	IsVenueMember(ctx context.Context, venueID, userID uuid.UUID) (bool, error)

	// StaffUserIDsForVenue lists staff user_ids for notification fan-out
	// (moderator emails, suspend/resume). Same legacy bridge as IsVenueMember.
	StaffUserIDsForVenue(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error)

	// nameQuery optional substring for admin list (ILIKE), empty = all names.
	ListByStatus(ctx context.Context, status string, page, pageSize int32, nameQuery string) (*ListResult, error)
	UpdateStatus(ctx context.Context, venueID uuid.UUID, status, comment string, moderatedBy uuid.UUID) error
	ResetToPendingReview(ctx context.Context, venueID uuid.UUID) error
	// SubmitDraftForReview sets status pending_review for venue in draft owned by ownerID. Returns false if no row updated.
	SubmitDraftForReview(ctx context.Context, venueID, ownerID uuid.UUID) (updated bool, err error)
	InsertModerationHistory(ctx context.Context, entry *ModerationHistoryEntry) error
	GetModerationHistory(ctx context.Context, venueID uuid.UUID) ([]ModerationHistoryEntry, error)
	UpdateRating(ctx context.Context, venueID uuid.UUID, avgRating float64, reviewCount int32) error
	// CheckSlot reports whether the interval is free for the reserved resource:
	// the given halls in per-hall mode, or the whole venue when hallIDs is empty.
	CheckSlot(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo string) (bool, error)
	// BatchCheckSlots checks multiple (time_from, time_to) intervals for the
	// same venue, halls and date. Returns one bool per slot in the same order.
	// Implementations should resolve all intervals in a single DB query.
	BatchCheckSlots(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date string, slots [][2]string) ([]bool, error)
	// ReserveSlot atomically reserves the interval. With no hallIDs it inserts a
	// single whole-venue row (hall_id NULL); with hallIDs it inserts one row per
	// hall in a single transaction. Any overlap yields ErrSlotUnavailable and no
	// rows are written.
	ReserveSlot(ctx context.Context, venueID, bookingID uuid.UUID, date, timeFrom, timeTo string, hallIDs []uuid.UUID) error
	ReleaseSlot(ctx context.Context, venueID, bookingID uuid.UUID) error
	// BookingMode returns the venue's booking mode ("whole" | "per_hall").
	BookingMode(ctx context.Context, venueID uuid.UUID) (string, error)
	// SetBookingMode updates the venue's booking mode.
	SetBookingMode(ctx context.Context, venueID uuid.UUID, mode string) error
	// HallIDs returns the ids of all halls defined for the venue.
	HallIDs(ctx context.Context, venueID uuid.UUID) ([]uuid.UUID, error)
	// CreateManualSlotBlock inserts a manual block per hall (one whole-venue row
	// with hall_id NULL when hallIDs is empty), atomically. Any overlap yields
	// ErrSlotUnavailable and no rows are written.
	CreateManualSlotBlock(ctx context.Context, venueID uuid.UUID, hallIDs []uuid.UUID, date, timeFrom, timeTo, note string) ([]ManualSlotBlock, error)
	DeleteManualSlotBlock(ctx context.Context, venueID, blockID uuid.UUID) (deleted bool, err error)
	ListManualSlotBlocks(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]ManualSlotBlock, error)
	// ListVenueSchedule returns every occupied interval — aggregator bookings and
	// manual owner blocks — for the venue in the inclusive date range, ordered by
	// date, time_from. Read-only source for the owner schedule grid.
	ListVenueSchedule(ctx context.Context, venueID uuid.UUID, dateFrom, dateTo string) ([]ScheduleEntry, error)

	AddVenuePhoto(ctx context.Context, venueID, ownerID uuid.UUID, url string) (*VenuePhoto, error)
	DeleteVenuePhoto(ctx context.Context, venueID, ownerID, photoID uuid.UUID) (deletedURL string, err error)
	SetVenueCoverPhoto(ctx context.Context, venueID, ownerID, photoID uuid.UUID) error

	ReplaceVenueHalls(ctx context.Context, venueID, ownerID uuid.UUID, items []VenueHallUpsert) error
	AddVenueHallPhoto(ctx context.Context, venueID, hallID uuid.UUID, url string) (*VenueHallPhoto, error)
	DeleteVenueHallPhoto(ctx context.Context, venueID, hallID, photoID uuid.UUID) (deletedURL string, err error)
	SetVenueHallCoverPhoto(ctx context.Context, venueID, hallID, photoID uuid.UUID) error
}
