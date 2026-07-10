package handler

import (
	"testing"

	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
)

func TestSocialLinksMapForJSON(t *testing.T) {
	if m := socialLinksMapForJSON(""); len(m) != 0 {
		t.Fatalf("empty -> %v", m)
	}
	if m := socialLinksMapForJSON("not json"); len(m) != 0 {
		t.Fatalf("invalid -> %v", m)
	}
	if m := socialLinksMapForJSON("null"); len(m) != 0 {
		t.Fatalf("null -> %v", m)
	}
	m := socialLinksMapForJSON(`{"vk":"https://vk.com/x","tg":"@y"}`)
	if m["vk"] != "https://vk.com/x" || m["tg"] != "@y" {
		t.Fatalf("valid -> %v", m)
	}
}

func TestSlotEndFromDuration(t *testing.T) {
	if got := slotEndFromDuration("10:00", 90); got != "11:30" {
		t.Fatalf("got %q", got)
	}
	if got := slotEndFromDuration("23:30", 60); got != "00:30" {
		t.Fatalf("wraparound got %q", got)
	}
	// invalid input is echoed back unchanged
	if got := slotEndFromDuration("bad", 30); got != "bad" {
		t.Fatalf("invalid got %q", got)
	}
}

func TestBookingCandidateStartTimes(t *testing.T) {
	got := bookingCandidateStartTimes()
	if got[0] != "10:00" {
		t.Fatalf("first = %q", got[0])
	}
	if got[len(got)-1] != "22:00" {
		t.Fatalf("last = %q", got[len(got)-1])
	}
	// 10:00..22:00 inclusive, 30-min step => 25 entries
	if len(got) != 25 {
		t.Fatalf("len = %d", len(got))
	}
	if got[1] != "10:30" {
		t.Fatalf("second = %q", got[1])
	}
}

func TestScheduleEntryToJSON(t *testing.T) {
	bid := "bk1"
	hid := "hall1"
	booking := scheduleEntryToJSON(&venuev1.ScheduleEntry{Id: "e1", Date: "2026-01-02", BookingId: &bid, HallId: &hid})
	if booking["kind"] != "booking" || booking["booking_id"] != "bk1" || booking["hall_id"] != "hall1" {
		t.Fatalf("booking entry = %v", booking)
	}
	if _, ok := booking["note"]; ok {
		t.Fatalf("booking entry should not carry note: %v", booking)
	}

	manual := scheduleEntryToJSON(&venuev1.ScheduleEntry{Id: "e2", Note: "closed"})
	if manual["kind"] != "manual_block" || manual["note"] != "closed" {
		t.Fatalf("manual entry = %v", manual)
	}
	if _, ok := manual["hall_id"]; ok {
		t.Fatalf("manual entry without hall should omit hall_id: %v", manual)
	}
}

func TestManualSlotBlockToJSON(t *testing.T) {
	if manualSlotBlockToJSON(nil) != nil {
		t.Fatal("nil -> want nil")
	}
	hid := "hall1"
	m := manualSlotBlockToJSON(&venuev1.ManualSlotBlock{Id: "b1", VenueId: "v1", Date: "2026-01-02", Note: "n", HallId: &hid})
	if m["id"] != "b1" || m["hall_id"] != "hall1" || m["note"] != "n" {
		t.Fatalf("block = %v", m)
	}
	noHall := manualSlotBlockToJSON(&venuev1.ManualSlotBlock{Id: "b2"})
	if _, ok := noHall["hall_id"]; ok {
		t.Fatalf("whole-venue block should omit hall_id: %v", noHall)
	}
}

func TestVenueHallItemsToProto(t *testing.T) {
	id := "  h-existing  "
	blank := "   "
	items := []venueHallItemReq{
		{Name: "  Parnaya  ", PriceFrom: 1000, Capacity: 4, SortOrder: 1}, // nil amenities -> []
		{ID: &id, Name: "VIP", Amenities: []string{"pool"}},               // trimmed id kept
		{ID: &blank, Name: "Blank ID"},                                    // blank id dropped
	}
	out := venueHallItemsToProto(items)
	if len(out) != 3 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].GetName() != "Parnaya" {
		t.Fatalf("name not trimmed: %q", out[0].GetName())
	}
	if out[0].GetAmenities() == nil {
		t.Fatal("nil amenities should become empty slice")
	}
	if out[1].GetId() != "h-existing" {
		t.Fatalf("id not trimmed/kept: %q", out[1].GetId())
	}
	if out[2].Id != nil {
		t.Fatalf("blank id should be dropped, got %v", out[2].Id)
	}
}
