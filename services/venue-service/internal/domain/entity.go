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
	PriceFrom         int64
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
	// NOTE: yookassa_seller_account_id was dropped when the platform moved from
	// marketplace-split to escrow + per-partner payouts.  The DB column lives on
	// (default '') until a follow-up migration removes it; venue-service simply
	// stops reading/writing it.  Partner payout rails now live in payment-service.
	Services []VenueService
	Photos            []VenuePhoto
	Halls             []VenueHall
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

// VenueHall is a rentable space inside a venue (Russian: зал).
type VenueHall struct {
	ID          uuid.UUID
	VenueID     uuid.UUID
	Name        string
	PriceFrom   int64
	Capacity    int32
	Amenities   []string
	SortOrder   int32
	Photos      []VenueHallPhoto
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
	ID        *uuid.UUID
	Name      string
	PriceFrom int64
	Capacity  int32
	Amenities []string
	SortOrder int32
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
	Date     string // YYYY-MM-DD
	TimeFrom string // HH:MM
	TimeTo   string // HH:MM
	Note     string
}
