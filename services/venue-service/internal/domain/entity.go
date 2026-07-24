package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusDraft         = "draft"
	StatusPendingReview = "pending_review"
	StatusActive        = "active"
	StatusRejected      = "rejected"
	StatusSuspended     = "suspended"

	VenueTypeBanya  = "banya"
	VenueTypeSauna  = "sauna"
	VenueTypeHammam = "hammam"

	MaxVenuePhotos = 24
	MaxVenueVideos = 6

	PayoutLegalFormEmpty        = ""
	PayoutLegalFormIP           = "ip"
	PayoutLegalFormOOO          = "ooo"
	PayoutLegalFormSelfEmployed = "self_employed"
	PayoutLegalFormGPH          = "gph"
)

type Venue struct {
	ID                uuid.UUID
	OwnerID           uuid.UUID
	Slug              string
	Name              string
	Type              string
	Description       string
	Address           string
	City              string
	Latitude          float64
	Longitude         float64
	PriceFrom         int64 // почасовая ставка в будни (копейки)
	PriceWeekend      int64 // почасовая ставка в выходные (копейки); 0 = как в будни
	Capacity          int32
	Amenities         []string
	WorkingHours      string
	Phone             string
	AvgRating         float64
	ReviewCount       int32
	IsActive          bool
	Status            string
	ModerationComment string
	ModeratedAt       *time.Time
	ModeratedBy       *uuid.UUID
	LegalEntityName   string
	INN               string
	OGRN              string
	PublicListingURL  string
	VerificationNote  string
	// SocialLinks is a JSON object string, e.g. {"vk":"https://vk.com/..."}; empty → "{}".
	SocialLinks string
	// PayoutLegalForm: ip | ooo | self_employed | gph (empty = not set).
	PayoutLegalForm string
	// NOTE: the legacy seller-account column was dropped when the platform moved
	// from marketplace-split to escrow + per-partner payouts (DB column removed in
	// migration 016).  Partner payout rails now live in payment-service.
	Services  []VenueService
	Photos    []VenuePhoto
	Videos    []VenueVideo
	Halls     []VenueHall
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ModerationHistoryEntry struct {
	ID        uuid.UUID
	VenueID   uuid.UUID
	OldStatus string
	NewStatus string
	Comment   string
	ChangedBy uuid.UUID
	CreatedAt time.Time
}

type VenueService struct {
	ID          uuid.UUID
	VenueID     uuid.UUID
	Name        string
	DurationMin int32
	Price       int64
	Description string
	SortOrder   int32
}

type VenuePhoto struct {
	ID        uuid.UUID
	VenueID   uuid.UUID
	URL       string
	SortOrder int32
	IsCover   bool
}

type VenueVideo struct {
	ID        uuid.UUID
	VenueID   uuid.UUID
	URL       string
	SortOrder int32
}

// VenueHall is a rentable space inside a venue (Russian: зал).
type VenueHall struct {
	ID           uuid.UUID
	VenueID      uuid.UUID
	Name         string
	PriceFrom    int64 // почасовая ставка в будни (копейки)
	PriceWeekend int64 // почасовая ставка в выходные (копейки); 0 = как в будни
	Capacity     int32
	Amenities    []string
	SortOrder    int32
	Description  string // свободное описание зала
	SteamType    string // тип парной: дровяная|электро|финская|хамам|инфракрасная
	Photos       []VenueHallPhoto
}

type VenueHallPhoto struct {
	ID        uuid.UUID
	HallID    uuid.UUID
	URL       string
	SortOrder int32
	IsCover   bool
}

// VenueHallUpsert is used to create or update halls (nil ID = insert).
type VenueHallUpsert struct {
	ID           *uuid.UUID
	Name         string
	PriceFrom    int64
	PriceWeekend int64
	Capacity     int32
	Amenities    []string
	SortOrder    int32
	Description  string
	SteamType    string
}

type ReservedSlot struct {
	ID        uuid.UUID
	VenueID   uuid.UUID
	BookingID uuid.UUID
	Date      string
	TimeFrom  string
	TimeTo    string
}

// ManualSlotBlock is a reserved interval without an aggregator booking (external / phone booking).
type ManualSlotBlock struct {
	ID       uuid.UUID
	VenueID  uuid.UUID
	HallID   *uuid.UUID // nil = whole-venue block (mode=whole)
	Date     string     // YYYY-MM-DD
	TimeFrom string     // HH:MM
	TimeTo   string     // HH:MM
	Note     string
}

// Booking modes control how a venue's availability is reserved.
//   - BookingModeWhole: one reservation blocks the whole venue (the default;
//     hall_id stays NULL).
//   - BookingModePerHall: each selected hall is reserved independently, so
//     different halls can be booked for the same interval.
const (
	BookingModeWhole   = "whole"
	BookingModePerHall = "per_hall"
)

// ScheduleEntry is one occupied interval on a venue's day schedule: either an
// aggregator booking (BookingID set) or a manual owner block (BookingID nil).
// Both kinds live in the shared reserved_slots table. HallID is set only for
// per-hall reservations; nil means the entry occupies the whole venue and
// belongs on an "all halls" row of the grid.
type ScheduleEntry struct {
	ID        uuid.UUID
	BookingID *uuid.UUID // nil = manual owner block
	HallID    *uuid.UUID // nil = whole-venue reservation (mode=whole)
	Date      string     // YYYY-MM-DD
	TimeFrom  string     // HH:MM
	TimeTo    string     // HH:MM
	Note      string     // manual block note; empty for aggregator bookings
}
