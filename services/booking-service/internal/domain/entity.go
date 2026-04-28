package domain

import "time"

type Booking struct {
	ID         string
	UserID     string
	VenueID    string
	VenueName  string
	ServiceID  string
	// PackageServiceIDs — несколько пакетов (из package_service_ids JSON); иначе пусто, см. ServiceID.
	PackageServiceIDs []string
	// HallIDs — выбранные залы (из booking_hall_ids JSON).
	HallIDs []string
	Date       time.Time
	TimeFrom   string
	TimeTo     string
	Guests     int32
	Comment    string
	Status     string
	TotalPrice int64
	PaymentID  string
	PaymentURL string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// BookingStaffNote is an internal venue note on a booking (CRM, not guest-visible).
type BookingStaffNote struct {
	ID           string
	BookingID    string
	VenueID      string
	AuthorUserID string
	Body         string
	CreatedAt    time.Time
}
