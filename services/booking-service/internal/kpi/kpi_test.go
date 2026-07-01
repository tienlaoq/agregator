package kpi

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBookingEvent_KnownEvents(t *testing.T) {
	bookings.Reset()

	BookingEvent("created")
	BookingEvent("confirmed")
	BookingEvent("cancelled")
	BookingEvent("completed")

	for _, e := range []string{"created", "confirmed", "cancelled", "completed"} {
		if got := testutil.ToFloat64(bookings.WithLabelValues(e)); got != 1 {
			t.Errorf("%s = %v, want 1", e, got)
		}
	}
}

func TestBookingEvents_UnknownClampedToOther(t *testing.T) {
	bookings.Reset()

	BookingEvents("weird", 3)
	BookingEvent("")

	if got := testutil.ToFloat64(bookings.WithLabelValues("other")); got != 4 {
		t.Errorf("other = %v, want 4 (3 + 1)", got)
	}
}

func TestBookingEvents_NonPositiveIsNoOp(t *testing.T) {
	bookings.Reset()

	BookingEvents("created", 0)
	BookingEvents("created", -2)

	if got := testutil.ToFloat64(bookings.WithLabelValues("created")); got != 0 {
		t.Errorf("created = %v, want 0 (non-positive n is a no-op)", got)
	}
}

func TestRegister(t *testing.T) {
	reg := prometheus.NewRegistry()
	Register(reg)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var found bool
	for _, mf := range mfs {
		if mf.GetName() == "booking_service_bookings_total" {
			found = true
		}
	}
	if !found {
		t.Error("booking_service_bookings_total not registered")
	}
}
