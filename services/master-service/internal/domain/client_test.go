package domain

import (
	"testing"
	"time"
)

func TestComputeClientSegments(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	old := now.Add(-100 * 24 * time.Hour) // beyond clientAtRiskAfter
	recent := now.Add(-10 * 24 * time.Hour)

	tests := []struct {
		name   string
		client MasterClient
		want   []string
	}{
		{
			name:   "single booking is new",
			client: MasterClient{BookingsCount: 1},
			want:   []string{ClientSegmentNew},
		},
		{
			name:   "zero bookings is new",
			client: MasterClient{BookingsCount: 0},
			want:   []string{ClientSegmentNew},
		},
		{
			name:   "three visits is regular",
			client: MasterClient{BookingsCount: 5, VisitsCount: 3, LastVisitAt: &recent},
			want:   []string{ClientSegmentRegular},
		},
		{
			name:   "two visits long ago is at_risk",
			client: MasterClient{BookingsCount: 4, VisitsCount: 2, LastVisitAt: &old},
			want:   []string{ClientSegmentAtRisk},
		},
		{
			name:   "regular and at_risk together",
			client: MasterClient{BookingsCount: 6, VisitsCount: 3, LastVisitAt: &old},
			want:   []string{ClientSegmentRegular, ClientSegmentAtRisk},
		},
		{
			name:   "returning recent visitor has no segment",
			client: MasterClient{BookingsCount: 2, VisitsCount: 2, LastVisitAt: &recent},
			want:   []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeClientSegments(&tt.client, now)
			if !equalStrings(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidClientSegment(t *testing.T) {
	valid := []string{ClientSegmentNew, ClientSegmentRegular, ClientSegmentAtRisk}
	for _, s := range valid {
		if !ValidClientSegment(s) {
			t.Errorf("ValidClientSegment(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "vip", "unknown"} {
		if ValidClientSegment(s) {
			t.Errorf("ValidClientSegment(%q) = true, want false", s)
		}
	}
}

func TestPayoutValidationError_Error(t *testing.T) {
	err := &PayoutValidationError{Field: "bik", Message: "некорректный БИК"}
	if err.Error() != "некорректный БИК" {
		t.Fatalf("Error() = %q, want the message", err.Error())
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
