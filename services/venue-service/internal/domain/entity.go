package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusPendingReview = "pending_review"
	StatusActive        = "active"
	StatusRejected      = "rejected"
	StatusSuspended     = "suspended"
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
	Services          []VenueService
	Photos            []VenuePhoto
	CreatedAt         time.Time
	UpdatedAt         time.Time
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
}

type VenuePhoto struct {
	ID        uuid.UUID
	VenueID   uuid.UUID
	URL       string
	SortOrder int32
	IsCover   bool
}

type ReservedSlot struct {
	ID        uuid.UUID
	VenueID   uuid.UUID
	BookingID uuid.UUID
	Date      string
	TimeFrom  string
	TimeTo    string
}
