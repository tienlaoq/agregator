// Package pricing holds small, shared helpers for weekday/weekend hourly rates.
// Both venue (booking-service) and master (master-service) pricing pick a rate by
// the booking day; keeping the rule in one place stops the two copies from drifting.
package pricing

import "time"

// IsWeekendDay reports whether t falls on Saturday or Sunday.
//
// A bare calendar date's weekday is timezone-independent, so callers may parse a
// "YYYY-MM-DD" booking date in any location (or UTC) before calling this.
func IsWeekendDay(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// WeekendRate returns the applicable hourly rate: the weekend rate when it is set
// (>0) and the day is a weekend, otherwise the weekday rate. A zero weekend rate
// therefore means "same as weekday".
func WeekendRate(weekday, weekend int64, isWeekend bool) int64 {
	if isWeekend && weekend > 0 {
		return weekend
	}
	return weekday
}
