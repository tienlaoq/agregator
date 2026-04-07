package domain

import "time"

type Booking struct {
	ID         string
	UserID     string
	VenueID    string
	ServiceID  string
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
