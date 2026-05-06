package domain

import "time"

type Review struct {
	ID          string
	UserID      string
	UserName    string
	VenueID     string
	MasterID    string
	Rating      int32
	Text        string
	IsVerified  bool
	IsAnonymous bool
	CreatedAt   time.Time
}

type VenueRating struct {
	VenueID     string
	AvgRating   float64
	ReviewCount int32
	UpdatedAt   time.Time
}
