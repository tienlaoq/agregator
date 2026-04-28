package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusDraft         = "draft"
	StatusPendingReview = "pending_review"
	StatusNeedsRevision = "needs_revision"
	StatusActive        = "active"
	StatusRejected      = "rejected"
	StatusSuspended     = "suspended"
)

const (
	WorkFormatVenue  = "venue"
	WorkFormatMobile = "mobile"
	WorkFormatBoth   = "both"
)

// Форма, по которой мастер принимает выплаты (ИП / ООО / физлицо / самозанятость).
const (
	PayoutLegalFormIP           = "ip"
	PayoutLegalFormOOO          = "ooo"
	PayoutLegalFormIndividual   = "individual"
	PayoutLegalFormSelfEmployed = "self_employed"
)

type Master struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	Slug              string
	DisplayName       string
	Bio               string
	Phone             string
	City              string
	WorkFormat        string
	TravelRadiusKm    int32
	// Точка отсчёта радиуса выезда (если WorkFormat mobile/both).
	TravelBaseLatitude  *float64
	TravelBaseLongitude *float64
	TravelExcludeZones  []MasterTravelExcludeZone
	ExperienceYears   int32
	Specializations   []string
	HourlyRate        int64
	AvailabilityJSON  string
	PayoutLegalForm   string
	Status            string
	ModerationComment string
	ModeratedBy       *uuid.UUID
	ModeratedAt       *time.Time
	Services          []MasterService
	Photos            []MasterPhoto
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type MasterPhoto struct {
	ID        uuid.UUID
	MasterID  uuid.UUID
	URL       string
	SortOrder int32
	IsCover   bool
}

type MasterService struct {
	ID          uuid.UUID
	MasterID    uuid.UUID
	Name        string
	Description string
	DurationMin int32
	Price       int64
	SortOrder   int32
}

// MasterTravelExcludeZone — круг на карте «сюда не выезжаю» (внутри общей зоны от метки).
type MasterTravelExcludeZone struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	RadiusKm  float64 `json:"radius_km"`
	Label     string  `json:"label,omitempty"`
}

type MasterServiceUpsert struct {
	ID          *uuid.UUID
	Name        string
	Description string
	DurationMin int32
	Price       int64
	SortOrder   int32
}

type ModerationHistoryEntry struct {
	ID        uuid.UUID
	MasterID  uuid.UUID
	OldStatus string
	NewStatus string
	Comment   string
	ChangedBy uuid.UUID
	CreatedAt time.Time
}

type MasterBooking struct {
	ID              uuid.UUID
	MasterID        uuid.UUID
	ClientUserID    uuid.UUID
	MasterServiceID *uuid.UUID
	Date            string
	TimeFrom        string
	TimeTo          string
	Comment         string
	Status          string
	CreatedAt       time.Time
}
