package domain

import (
	"time"

	"github.com/google/uuid"
)

type Venue struct {
	ID           uuid.UUID
	OwnerID      uuid.UUID
	Slug         string
	Name         string
	Type         string
	Description  string
	Address      string
	Latitude     float64
	Longitude    float64
	PriceFrom    int64
	Capacity     int32
	Amenities    []string
	WorkingHours string
	Phone        string
	AvgRating    float64
	ReviewCount  int32
	IsActive     bool
	Services     []VenueService
	Photos       []VenuePhoto
	CreatedAt    time.Time
	UpdatedAt    time.Time
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
