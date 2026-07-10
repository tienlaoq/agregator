package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// bookingStub is a flexible BookingServiceClient mock via interface embedding.
type bookingStub struct {
	bookingv1.BookingServiceClient
	onCreate    func(*bookingv1.CreateBookingRequest) (*bookingv1.BookingResponse, error)
	onCancel    func(*bookingv1.CancelBookingRequest) (*bookingv1.BookingResponse, error)
	onListVenue func(*bookingv1.ListVenueBookingsRequest) (*bookingv1.ListBookingsResponse, error)
	onListNotes func(*bookingv1.ListBookingStaffNotesRequest) (*bookingv1.ListBookingStaffNotesResponse, error)
	onAddNote   func(*bookingv1.AddBookingStaffNoteRequest) (*bookingv1.AddBookingStaffNoteResponse, error)
}

func (b *bookingStub) CreateBooking(_ context.Context, in *bookingv1.CreateBookingRequest, _ ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if b.onCreate != nil {
		return b.onCreate(in)
	}
	return &bookingv1.BookingResponse{Id: "bk1", CreatedAt: timestamppb.Now()}, nil
}
func (b *bookingStub) CancelBooking(_ context.Context, in *bookingv1.CancelBookingRequest, _ ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if b.onCancel != nil {
		return b.onCancel(in)
	}
	return &bookingv1.BookingResponse{Id: in.GetId(), Status: "cancelled", CreatedAt: timestamppb.Now()}, nil
}
func (b *bookingStub) ListVenueBookings(_ context.Context, in *bookingv1.ListVenueBookingsRequest, _ ...grpc.CallOption) (*bookingv1.ListBookingsResponse, error) {
	if b.onListVenue != nil {
		return b.onListVenue(in)
	}
	return &bookingv1.ListBookingsResponse{}, nil
}
func (b *bookingStub) ListBookingStaffNotes(_ context.Context, in *bookingv1.ListBookingStaffNotesRequest, _ ...grpc.CallOption) (*bookingv1.ListBookingStaffNotesResponse, error) {
	if b.onListNotes != nil {
		return b.onListNotes(in)
	}
	return &bookingv1.ListBookingStaffNotesResponse{}, nil
}
func (b *bookingStub) AddBookingStaffNote(_ context.Context, in *bookingv1.AddBookingStaffNoteRequest, _ ...grpc.CallOption) (*bookingv1.AddBookingStaffNoteResponse, error) {
	if b.onAddNote != nil {
		return b.onAddNote(in)
	}
	return &bookingv1.AddBookingStaffNoteResponse{Note: &bookingv1.BookingStaffNote{Id: "n1", CreatedAt: timestamppb.Now()}}, nil
}

type bookingNotifierSpy struct {
	calls   int
	ownerID string
}

func (s *bookingNotifierSpy) NotifyVenueBookingCreated(_ context.Context, ownerID, _, _, _, _, _, _ string, _ int32) {
	s.calls++
	s.ownerID = ownerID
}

// --- Create notifier behaviour ---

func TestBookingCreate_FiresOwnerNotifier(t *testing.T) {
	spy := &bookingNotifierSpy{}
	venue := &reviewVenueMock{onGet: func(id string) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: id, Name: "Баня", OwnerId: "owner-1"}, nil
	}}
	h := NewBookingHandler(&bookingStub{}, venue, &reviewUserMock{}, WithBookingOwnerNotifier(spy))
	rr := httptest.NewRecorder()
	body := `{"venue_id":"v1","date":"2026-01-02","time_from":"10:00","guests":2}`
	h.Create(rr, reviewReq(http.MethodPost, "/bookings", body, nil, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if spy.calls != 1 || spy.ownerID != "owner-1" {
		t.Fatalf("notifier: calls=%d owner=%s", spy.calls, spy.ownerID)
	}
}

func TestBookingCreate_SelfBookingNoNotify(t *testing.T) {
	spy := &bookingNotifierSpy{}
	venue := &reviewVenueMock{onGet: func(id string) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: id, OwnerId: "u1"}, nil // owner == caller
	}}
	h := NewBookingHandler(&bookingStub{}, venue, &reviewUserMock{}, WithBookingOwnerNotifier(spy))
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/bookings", `{"venue_id":"v1","date":"2026-01-02","time_from":"10:00"}`, nil, "u1"))
	if spy.calls != 0 {
		t.Fatalf("expected no self-booking notify, got %d", spy.calls)
	}
}

func TestBookingCreate_DefaultsTimeToTwoHours(t *testing.T) {
	var got *bookingv1.CreateBookingRequest
	stub := &bookingStub{onCreate: func(in *bookingv1.CreateBookingRequest) (*bookingv1.BookingResponse, error) {
		got = in
		return &bookingv1.BookingResponse{Id: "bk1", CreatedAt: timestamppb.Now()}, nil
	}}
	h := NewBookingHandler(stub, &reviewVenueMock{}, &reviewUserMock{})
	rr := httptest.NewRecorder()
	// uses "time" alias for time_from and omits time_to
	h.Create(rr, reviewReq(http.MethodPost, "/bookings", `{"venue_id":"v1","date":"2026-01-02","time":"10:00"}`, nil, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.GetTimeFrom() != "10:00" || got.GetTimeTo() != "12:00" {
		t.Fatalf("time_from=%s time_to=%s", got.GetTimeFrom(), got.GetTimeTo())
	}
}

// --- Cancel ---

func TestBookingCancel_Unauthorized(t *testing.T) {
	h := NewBookingHandler(&bookingStub{}, &reviewVenueMock{}, &reviewUserMock{})
	rr := httptest.NewRecorder()
	h.Cancel(rr, reviewReq(http.MethodPost, "/bookings/bk1/cancel", "", map[string]string{"id": "bk1"}, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestBookingCancel_Success(t *testing.T) {
	var got *bookingv1.CancelBookingRequest
	stub := &bookingStub{onCancel: func(in *bookingv1.CancelBookingRequest) (*bookingv1.BookingResponse, error) {
		got = in
		return &bookingv1.BookingResponse{Id: in.GetId(), Status: "cancelled", CreatedAt: timestamppb.Now()}, nil
	}}
	h := NewBookingHandler(stub, &reviewVenueMock{}, &reviewUserMock{})
	rr := httptest.NewRecorder()
	h.Cancel(rr, reviewReq(http.MethodPost, "/bookings/bk1/cancel", "", map[string]string{"id": "bk1"}, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.GetId() != "bk1" || got.GetUserId() != "u1" {
		t.Fatalf("cancel req = %+v", got)
	}
}

// --- ListVenueBookings enriches names + cursor ---

func TestBookingListVenue_EnrichesNamesAndCursor(t *testing.T) {
	stub := &bookingStub{onListVenue: func(in *bookingv1.ListVenueBookingsRequest) (*bookingv1.ListBookingsResponse, error) {
		if in.GetVenueId() != "v1" || in.GetOwnerId() != "owner1" {
			t.Fatalf("req = %+v", in)
		}
		return &bookingv1.ListBookingsResponse{
			Total:      1,
			NextCursor: "next-token",
			Bookings:   []*bookingv1.BookingResponse{{Id: "bk1", UserId: "c1", CreatedAt: timestamppb.Now()}},
		}, nil
	}}
	user := &reviewUserMock{onGet: func(id string) (*userv1.UserResponse, error) {
		return &userv1.UserResponse{Id: id, Name: "Guest One"}, nil
	}}
	h := NewBookingHandler(stub, &reviewVenueMock{}, user)
	rr := httptest.NewRecorder()
	h.ListVenueBookings(rr, reviewReq(http.MethodGet, "/venues/v1/bookings?page_size=20", "", map[string]string{"venueId": "v1"}, "owner1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Bookings   []map[string]any `json:"bookings"`
		NextCursor string           `json:"next_cursor"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.NextCursor != "next-token" {
		t.Fatalf("next_cursor = %s", body.NextCursor)
	}
	if body.Bookings[0]["user_name"] != "Guest One" {
		t.Fatalf("user_name = %v", body.Bookings[0]["user_name"])
	}
}

func TestBookingListVenue_Error(t *testing.T) {
	stub := &bookingStub{onListVenue: func(*bookingv1.ListVenueBookingsRequest) (*bookingv1.ListBookingsResponse, error) {
		return nil, status.Error(codes.PermissionDenied, "not owner")
	}}
	h := NewBookingHandler(stub, &reviewVenueMock{}, &reviewUserMock{})
	rr := httptest.NewRecorder()
	h.ListVenueBookings(rr, reviewReq(http.MethodGet, "/venues/v1/bookings", "", map[string]string{"venueId": "v1"}, "owner1"))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- staff notes ---

func TestBookingListStaffNotes_Success(t *testing.T) {
	stub := &bookingStub{onListNotes: func(in *bookingv1.ListBookingStaffNotesRequest) (*bookingv1.ListBookingStaffNotesResponse, error) {
		return &bookingv1.ListBookingStaffNotesResponse{Notes: []*bookingv1.BookingStaffNote{
			{Id: "n1", BookingId: in.GetBookingId(), Body: "note", CreatedAt: timestamppb.Now()},
		}}, nil
	}}
	h := NewBookingHandler(stub, &reviewVenueMock{}, &reviewUserMock{})
	rr := httptest.NewRecorder()
	h.ListBookingStaffNotes(rr, reviewReq(http.MethodGet, "/bookings/bk1/notes", "", map[string]string{"bookingId": "bk1"}, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Notes []map[string]any `json:"notes"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Notes) != 1 || body.Notes[0]["body"] != "note" {
		t.Fatalf("notes = %v", body.Notes)
	}
}

func TestBookingAddStaffNote_Success(t *testing.T) {
	var got *bookingv1.AddBookingStaffNoteRequest
	stub := &bookingStub{onAddNote: func(in *bookingv1.AddBookingStaffNoteRequest) (*bookingv1.AddBookingStaffNoteResponse, error) {
		got = in
		return &bookingv1.AddBookingStaffNoteResponse{Note: &bookingv1.BookingStaffNote{Id: "n1", Body: in.GetBody(), CreatedAt: timestamppb.Now()}}, nil
	}}
	h := NewBookingHandler(stub, &reviewVenueMock{}, &reviewUserMock{})
	rr := httptest.NewRecorder()
	h.AddBookingStaffNote(rr, reviewReq(http.MethodPost, "/bookings/bk1/notes", `{"body":"call guest"}`, map[string]string{"bookingId": "bk1"}, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.GetBody() != "call guest" || got.GetRequesterUserId() != "u1" {
		t.Fatalf("add note req = %+v", got)
	}
}

func TestBookingAddStaffNote_Unauthorized(t *testing.T) {
	h := NewBookingHandler(&bookingStub{}, &reviewVenueMock{}, &reviewUserMock{})
	rr := httptest.NewRecorder()
	h.AddBookingStaffNote(rr, reviewReq(http.MethodPost, "/bookings/bk1/notes", `{"body":"x"}`, map[string]string{"bookingId": "bk1"}, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- bookingToJSON optional fields ---

func TestBookingToJSON_OptionalFields(t *testing.T) {
	full := bookingToJSON(&bookingv1.BookingResponse{
		Id: "bk1", TimeFrom: "10:00", TimeTo: "12:00",
		PackageServiceIds: []string{"s1", "s2"}, HallIds: []string{"h1"},
		PaymentUrl: "http://pay", StaffNotesCount: 3, CreatedAt: timestamppb.Now(),
	})
	if full["time"] != "10:00–12:00" {
		t.Fatalf("time display = %v", full["time"])
	}
	for _, k := range []string{"package_service_ids", "hall_ids", "payment_url", "staff_notes_count"} {
		if _, ok := full[k]; !ok {
			t.Fatalf("missing optional %q", k)
		}
	}
	// minimal: optionals omitted, time == time_from only
	minimal := bookingToJSON(&bookingv1.BookingResponse{Id: "bk2", TimeFrom: "09:00", CreatedAt: timestamppb.Now()})
	if minimal["time"] != "09:00" {
		t.Fatalf("time = %v", minimal["time"])
	}
	for _, k := range []string{"package_service_ids", "hall_ids", "payment_url", "staff_notes_count"} {
		if _, ok := minimal[k]; ok {
			t.Fatalf("unexpected optional %q on minimal", k)
		}
	}
}
