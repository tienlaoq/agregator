package domain

import "time"

// Review is a user-submitted rating and text for either a venue or a master.
// Exactly one of VenueID and MasterID is non-empty — this is enforced at the
// database level by the reviews_exactly_one_target check constraint and at the
// usecase level before the insert.
// IsVerified is true when the platform confirmed the user had a completed
// booking with the target before the review was submitted.
type Review struct {
	ID          string
	UserID      string
	UserName    string
	VenueID     string // non-empty when the review targets a venue
	MasterID    string // non-empty when the review targets a master
	Rating      int32  // 1–5 inclusive
	Text        string
	IsVerified  bool
	IsAnonymous bool
	CreatedAt   time.Time
}

// VenueRating is the denormalised rating summary for a venue, maintained by
// ratings_cache. It is updated incrementally on each new review so reads are
// O(1) without a full aggregate scan.
type VenueRating struct {
	VenueID     string
	AvgRating   float64
	ReviewCount int32
	UpdatedAt   time.Time
}

// MasterRating is the denormalised rating summary for a master, maintained by
// master_ratings_cache. Semantics are identical to VenueRating.
type MasterRating struct {
	MasterID    string
	AvgRating   float64
	ReviewCount int32
	UpdatedAt   time.Time
}
