package domain

import (
	"time"

	"github.com/google/uuid"
)

// Guest segments are computed from a profile (never stored). A guest may match
// several at once. See docs/CRM_DESIGN.md §5.
const (
	SegmentNew     = "new"
	SegmentRegular = "regular"
	SegmentVIP     = "vip"
	SegmentAtRisk  = "at_risk"
)

// atRiskAfter — a returning guest (>=2 visits) becomes "at risk" once this long
// passes since their last visit.
const atRiskAfter = 90 * 24 * time.Hour

// GuestProfile is a per-venue behavioural projection of one guest, aggregated
// from booking.* events. It holds only UUIDs + counters — no PII; the gateway
// enriches name/email from user-service on read (152-ФЗ: see docs/CRM_DESIGN.md §4.2).
type GuestProfile struct {
	VenueID            uuid.UUID
	UserID             uuid.UUID
	BookingsCount      int32
	VisitsCount        int32
	CancellationsCount int32
	NoShowCount        int32
	TotalSpent         int64
	FirstVisitAt       *time.Time
	LastVisitAt        *time.Time
	LastBookingAt      *time.Time
}

// GuestBookingSummary is a compact booking row for the 360 timeline.
type GuestBookingSummary struct {
	BookingID  uuid.UUID
	Status     string
	TotalPrice int64
	VisitDate  *time.Time
	Guests     int32
}

// BookingFact is one booking's contribution to the guest projection, derived
// from a booking.* event and upserted idempotently keyed by BookingID.
type BookingFact struct {
	BookingID  uuid.UUID
	VenueID    uuid.UUID
	UserID     uuid.UUID
	Status     string // pending | payment_pending | confirmed | completed | cancelled
	StatusRank int16
	TotalPrice int64
	VisitDate  *time.Time
	Guests     int32
}

// GuestListParams carries the filter/sort/pagination policy for ListGuests.
// VIPThreshold is in the same unit as GuestProfile.TotalSpent; <= 0 disables
// the VIP segment (both in the filter and in ComputeSegments).
type GuestListParams struct {
	Segment      string // "" | new | regular | vip | at_risk
	Sort         string // "" (=last_visit) | last_visit | ltv | visits
	Limit        int
	Offset       int
	VIPThreshold int64
}

// BookingStatusRank orders the booking lifecycle so out-of-order event delivery
// never regresses a fact to an earlier state. Terminal states (completed,
// cancelled) share the top rank; a booking is only ever one of them.
func BookingStatusRank(status string) int16 {
	switch status {
	case "completed", "cancelled":
		return 3
	case "confirmed":
		return 2
	default: // pending, payment_pending, or unknown
		return 1
	}
}

// ComputeSegments derives a guest's segment labels as of now. vipThreshold <= 0
// disables the VIP segment.
func ComputeSegments(p *GuestProfile, now time.Time, vipThreshold int64) []string {
	segs := make([]string, 0, 2)
	if p.BookingsCount <= 1 {
		segs = append(segs, SegmentNew)
	}
	if p.VisitsCount >= 3 {
		segs = append(segs, SegmentRegular)
	}
	if vipThreshold > 0 && p.TotalSpent >= vipThreshold {
		segs = append(segs, SegmentVIP)
	}
	if p.VisitsCount >= 2 && p.LastVisitAt != nil && now.Sub(*p.LastVisitAt) > atRiskAfter {
		segs = append(segs, SegmentAtRisk)
	}
	return segs
}

// ValidGuestSegment reports whether s is a known segment filter value.
func ValidGuestSegment(s string) bool {
	switch s {
	case SegmentNew, SegmentRegular, SegmentVIP, SegmentAtRisk:
		return true
	default:
		return false
	}
}
