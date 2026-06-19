package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBookingStatusRank(t *testing.T) {
	tests := map[string]int16{
		"pending":         1,
		"payment_pending": 1,
		"weird":           1,
		"confirmed":       2,
		"completed":       3,
		"cancelled":       3,
	}
	for status, want := range tests {
		if got := BookingStatusRank(status); got != want {
			t.Errorf("BookingStatusRank(%q) = %d, want %d", status, got, want)
		}
	}
}

func TestComputeSegments(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	long := now.Add(-100 * 24 * time.Hour) // beyond 90d → at-risk window
	recent := now.Add(-10 * 24 * time.Hour)
	vid, uid := uuid.New(), uuid.New()

	mk := func(bookings, visits int32, spent int64, last *time.Time) *GuestProfile {
		return &GuestProfile{
			VenueID: vid, UserID: uid,
			BookingsCount: bookings, VisitsCount: visits, TotalSpent: spent, LastVisitAt: last,
		}
	}
	has := func(segs []string, want string) bool {
		for _, s := range segs {
			if s == want {
				return true
			}
		}
		return false
	}

	t.Run("new", func(t *testing.T) {
		segs := ComputeSegments(mk(1, 0, 0, nil), now, 10000)
		if !has(segs, SegmentNew) || has(segs, SegmentRegular) {
			t.Errorf("got %v, want [new]", segs)
		}
	})

	t.Run("regular and vip", func(t *testing.T) {
		segs := ComputeSegments(mk(5, 4, 20000, &recent), now, 10000)
		if !has(segs, SegmentRegular) || !has(segs, SegmentVIP) {
			t.Errorf("got %v, want regular+vip", segs)
		}
		if has(segs, SegmentNew) || has(segs, SegmentAtRisk) {
			t.Errorf("got %v, did not want new/at_risk", segs)
		}
	})

	t.Run("vip disabled when threshold non-positive", func(t *testing.T) {
		segs := ComputeSegments(mk(5, 4, 999999, &recent), now, 0)
		if has(segs, SegmentVIP) {
			t.Errorf("got %v, vip must be disabled", segs)
		}
	})

	t.Run("at risk", func(t *testing.T) {
		segs := ComputeSegments(mk(3, 2, 0, &long), now, 10000)
		if !has(segs, SegmentAtRisk) {
			t.Errorf("got %v, want at_risk", segs)
		}
	})

	t.Run("recent returning guest is not at risk", func(t *testing.T) {
		segs := ComputeSegments(mk(3, 2, 0, &recent), now, 10000)
		if has(segs, SegmentAtRisk) {
			t.Errorf("got %v, must not be at_risk", segs)
		}
	})
}

func TestValidGuestSegment(t *testing.T) {
	for _, s := range []string{SegmentNew, SegmentRegular, SegmentVIP, SegmentAtRisk} {
		if !ValidGuestSegment(s) {
			t.Errorf("ValidGuestSegment(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "vp", "loyal"} {
		if ValidGuestSegment(s) {
			t.Errorf("ValidGuestSegment(%q) = true, want false", s)
		}
	}
}
