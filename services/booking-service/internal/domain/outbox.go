package domain

import "encoding/json"

// OutboxEvent is a domain event staged in the transactional outbox for
// at-least-once delivery to NATS. It is written in the same DB transaction as
// the state change that produced it, then published by a background poller.
// See TECH_DEBT [BOOKING-PUBLISH-LOSS].
type OutboxEvent struct {
	ID      int64
	Subject string
	Payload []byte
}

// bookingEventPayload is the canonical wire format for booking.* events.
// Downstream consumers (review-service, notification-service, crm-service)
// depend on these JSON field names — keep them stable. Fields are additive:
// total_price/date/guests were added for the CRM guest projection (Customer
// 360); older consumers simply ignore them.
type bookingEventPayload struct {
	BookingID  string `json:"booking_id"`
	UserID     string `json:"user_id"`
	VenueID    string `json:"venue_id"`
	Status     string `json:"status"`
	TotalPrice int64  `json:"total_price"` // for guest LTV
	Date       string `json:"date"`        // visit date, YYYY-MM-DD
	Guests     int32  `json:"guests"`      // party size
}

// NewBookingEvent builds an OutboxEvent for a booking.* subject. The payload is
// byte-compatible with what the direct publisher emits, so the poller can send
// it verbatim. This is the single source of truth for the event wire format:
// both the outbox writers and events.Publisher marshal through it.
func NewBookingEvent(subject string, b *Booking) (OutboxEvent, error) {
	payload, err := json.Marshal(bookingEventPayload{
		BookingID:  b.ID,
		UserID:     b.UserID,
		VenueID:    b.VenueID,
		Status:     string(b.Status),
		TotalPrice: b.TotalPrice,
		Date:       b.Date.Format("2006-01-02"),
		Guests:     b.Guests,
	})
	if err != nil {
		return OutboxEvent{}, err
	}
	return OutboxEvent{Subject: subject, Payload: payload}, nil
}
