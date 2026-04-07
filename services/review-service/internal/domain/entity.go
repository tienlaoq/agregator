package domain

import "time"

type Review struct {
	ID         string
	UserID     string
	VenueID    string
	Rating     int32
	Text       string
	IsVerified bool
	CreatedAt  time.Time
}

type VenueRating struct {
	VenueID     string
	AvgRating   float64
	ReviewCount int32
	UpdatedAt   time.Time
}
