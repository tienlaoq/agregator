package domain

import (
	"time"

	"github.com/google/uuid"
)

// Client segments are computed from a projection (never stored). A client may
// match several at once. They mirror the venue-CRM guest segments, minus VIP
// (a solo master has no configured spend threshold).
const (
	ClientSegmentNew     = "new"
	ClientSegmentRegular = "regular"
	ClientSegmentAtRisk  = "at_risk"
)

// clientAtRiskAfter — a returning client (>=2 visits) becomes "at risk" once
// this long passes since their last visit.
const clientAtRiskAfter = 90 * 24 * time.Hour

// MasterClient is a per-master behavioural projection of one client, aggregated
// on read from master_bookings. It holds only the client's UUID plus counters —
// no PII; the gateway enriches the display name from user-service on read.
type MasterClient struct {
	UserID        uuid.UUID
	BookingsCount int32
	VisitsCount   int32
	// TotalSpent is in kopecks, summed over visited bookings only.
	TotalSpent    int64
	FirstVisitAt  *time.Time
	LastVisitAt   *time.Time
	LastBookingAt *time.Time
	// Segments is filled by the usecase via ComputeClientSegments; the repository
	// leaves it nil.
	Segments []string
}

// MasterClientListParams carries the filter/sort/pagination policy for
// ListMyClients. Segment filtering and sorting are applied in the usecase
// because segments are computed in Go, not stored.
type MasterClientListParams struct {
	Segment string // "" | new | regular | at_risk
	Sort    string // "" (=last_visit) | last_visit | ltv | visits
	Limit   int
	Offset  int
}

// ComputeClientSegments derives a client's segment labels as of now.
func ComputeClientSegments(c *MasterClient, now time.Time) []string {
	segs := make([]string, 0, 2)
	if c.BookingsCount <= 1 {
		segs = append(segs, ClientSegmentNew)
	}
	if c.VisitsCount >= 3 {
		segs = append(segs, ClientSegmentRegular)
	}
	if c.VisitsCount >= 2 && c.LastVisitAt != nil && now.Sub(*c.LastVisitAt) > clientAtRiskAfter {
		segs = append(segs, ClientSegmentAtRisk)
	}
	return segs
}

// ValidClientSegment reports whether s is a known segment filter value.
func ValidClientSegment(s string) bool {
	switch s {
	case ClientSegmentNew, ClientSegmentRegular, ClientSegmentAtRisk:
		return true
	default:
		return false
	}
}
