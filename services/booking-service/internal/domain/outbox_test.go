package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewBookingEvent(t *testing.T) {
	b := &Booking{
		ID: "b1", UserID: "u1", VenueID: "v1", Status: StatusCompleted,
		TotalPrice: 4500, Guests: 4,
		Date: time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC),
	}

	ev, err := NewBookingEvent("booking.completed", b)
	if err != nil {
		t.Fatalf("NewBookingEvent: %v", err)
	}
	if ev.Subject != "booking.completed" {
		t.Errorf("subject = %q, want booking.completed", ev.Subject)
	}

	// Payload must be the canonical wire format consumers depend on.
	var got map[string]any
	if err := json.Unmarshal(ev.Payload, &got); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	want := map[string]any{
		"booking_id":  "b1",
		"user_id":     "u1",
		"venue_id":    "v1",
		"status":      "completed",
		"total_price": float64(4500), // JSON numbers decode to float64
		"date":        "2026-03-14",
		"guests":      float64(4),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("payload[%q] = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("payload has %d fields, want %d: %v", len(got), len(want), got)
	}
}
