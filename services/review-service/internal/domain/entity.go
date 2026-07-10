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
	Reply       *ReviewReply // non-nil when the venue owner has replied
}

// ReviewReply is the venue owner's public response to a review. At most one
// reply exists per review (review_replies.review_id is the primary key).
type ReviewReply struct {
	Body         string
	AuthorUserID string
	CreatedAt    time.Time
	UpdatedAt    time.Time
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

// VenueReviewSummary is the owner-dashboard header aggregate: the cached
// average/count plus how many reviews still await an owner reply.
type VenueReviewSummary struct {
	VenueID         string
	AvgRating       float64
	ReviewCount     int32
	UnansweredCount int32
}

// MasterRating is the denormalised rating summary for a master, maintained by
// master_ratings_cache. Semantics are identical to VenueRating.
type MasterRating struct {
	MasterID    string
	AvgRating   float64
	ReviewCount int32
	UpdatedAt   time.Time
}
