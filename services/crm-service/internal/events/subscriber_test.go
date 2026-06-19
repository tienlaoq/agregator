package events

import (
	"testing"

	"github.com/google/uuid"

	"github.com/tienlao/agregator/services/crm-service/internal/domain"
)

func TestBookingEventToFact(t *testing.T) {
	bookingID, venueID, userID := uuid.New(), uuid.New(), uuid.New()

	t.Run("completed event maps fully", func(t *testing.T) {
		f, err := bookingEventToFact(&bookingEvent{
			BookingID: bookingID.String(), VenueID: venueID.String(), UserID: userID.String(),
			Status: "completed", TotalPrice: 4500, Date: "2026-03-14", Guests: 4,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.BookingID != bookingID || f.VenueID != venueID || f.UserID != userID {
			t.Error("ids not mapped")
		}
		if f.Status != "completed" || f.StatusRank != domain.BookingStatusRank("completed") {
			t.Errorf("status/rank = %q/%d", f.Status, f.StatusRank)
		}
		if f.TotalPrice != 4500 || f.Guests != 4 {
			t.Errorf("price/guests = %d/%d", f.TotalPrice, f.Guests)
		}
		if f.VisitDate == nil || f.VisitDate.Format("2006-01-02") != "2026-03-14" {
			t.Errorf("visit date = %v", f.VisitDate)
		}
	})

	t.Run("empty date yields nil visit", func(t *testing.T) {
		f, err := bookingEventToFact(&bookingEvent{
			BookingID: bookingID.String(), VenueID: venueID.String(), UserID: userID.String(),
			Status: "pending",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.VisitDate != nil {
			t.Errorf("visit date = %v, want nil", f.VisitDate)
		}
	})

	t.Run("bad uuid errors", func(t *testing.T) {
		_, err := bookingEventToFact(&bookingEvent{
			BookingID: "not-a-uuid", VenueID: venueID.String(), UserID: userID.String(),
		})
		if err == nil {
			t.Error("expected error for malformed booking_id")
		}
	})
}
