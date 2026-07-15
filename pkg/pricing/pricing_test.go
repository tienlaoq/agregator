package pricing

import (
	"testing"
	"time"
)

func TestIsWeekendDay(t *testing.T) {
	// 2026-07-15 Wed .. 2026-07-19 Sun
	cases := map[string]bool{
		"2026-07-15": false, // Wed
		"2026-07-17": false, // Fri
		"2026-07-18": true,  // Sat
		"2026-07-19": true,  // Sun
		"2026-07-20": false, // Mon
	}
	for date, want := range cases {
		d, err := time.Parse(time.DateOnly, date)
		if err != nil {
			t.Fatalf("parse %s: %v", date, err)
		}
		if got := IsWeekendDay(d); got != want {
			t.Errorf("IsWeekendDay(%s) = %v, want %v", date, got, want)
		}
	}
}

func TestWeekendRate(t *testing.T) {
	tests := []struct {
		name             string
		weekday, weekend int64
		isWeekend        bool
		want             int64
	}{
		{"weekday", 3000, 4000, false, 3000},
		{"weekend uses weekend rate", 3000, 4000, true, 4000},
		{"weekend falls back when unset", 3000, 0, true, 3000},
		{"weekday ignores weekend rate", 3000, 4000, false, 3000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WeekendRate(tt.weekday, tt.weekend, tt.isWeekend); got != tt.want {
				t.Errorf("WeekendRate(%d,%d,%v) = %d, want %d", tt.weekday, tt.weekend, tt.isWeekend, got, tt.want)
			}
		})
	}
}
