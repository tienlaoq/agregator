package handler

import (
	"context"
	"sync"
	"testing"

	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type countingBookingClient struct {
	mu       sync.Mutex
	getCalls int
	byID     map[string]*bookingv1.BookingResponse
}

func (c *countingBookingClient) GetBooking(_ context.Context, in *bookingv1.GetBookingRequest, _ ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	c.mu.Lock()
	c.getCalls++
	c.mu.Unlock()
	if c.byID == nil {
		return nil, nil
	}
	return c.byID[in.GetId()], nil
}

type countingVenueClient struct {
	mu       sync.Mutex
	getCalls int
	byID     map[string]*venuev1.VenueResponse
}

func (c *countingVenueClient) GetVenue(_ context.Context, in *venuev1.GetVenueRequest, _ ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	c.mu.Lock()
	c.getCalls++
	c.mu.Unlock()
	if c.byID == nil {
		return nil, nil
	}
	return c.byID[in.GetId()], nil
}

func (c *countingVenueClient) GetVenueManagementAccess(context.Context, *venuev1.GetVenueManagementAccessRequest, ...grpc.CallOption) (*venuev1.GetVenueManagementAccessResponse, error) {
	return nil, nil
}
func (c *countingVenueClient) ListVenueStaff(context.Context, *venuev1.ListVenueStaffRequest, ...grpc.CallOption) (*venuev1.ListVenueStaffResponse, error) {
	return &venuev1.ListVenueStaffResponse{}, nil
}

type countingMasterClient struct {
	mu       sync.Mutex
	getCalls int
	byRef    map[string]*masterv1.MasterBooking
}

func (c *countingMasterClient) GetMasterBooking(_ context.Context, in *masterv1.GetMasterBookingRequest, _ ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	c.mu.Lock()
	c.getCalls++
	c.mu.Unlock()
	bk := c.byRef[in.GetBookingId()]
	if bk == nil {
		return &masterv1.MasterBookingResponse{}, nil
	}
	return &masterv1.MasterBookingResponse{Booking: bk}, nil
}

type staticUserClient struct {
	byID map[string]*userv1.UserResponse
}

func (c *staticUserClient) GetUser(_ context.Context, in *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	if c == nil || c.byID == nil {
		return nil, nil
	}
	return c.byID[in.GetId()], nil
}

// TestPeerDisplayNamesBatch — table-driven сценарии для резолва peer_display_name (бывший peerDisplayNameForThread).
func TestPeerDisplayNamesBatch(t *testing.T) {
	ctx := context.Background()

	t.Run("venue_booking", func(t *testing.T) {
		venueCases := []struct {
			name           string
			threads        []*chatv1.ChatThread
			booking        *countingBookingClient
			venue          *countingVenueClient
			users          *staticUserClient
			viewer         string
			want           map[string]string
			wantBookCalls  int
			wantVenueCalls int
		}{
			{
				name: "partner_sees_client_name_same_booking_twice_one_GetBooking",
				booking: &countingBookingClient{byID: map[string]*bookingv1.BookingResponse{
					"booking-1": {Id: "booking-1", VenueId: "v1", UserId: "u-client", VenueName: "Баня Лесная"},
				}},
				venue: &countingVenueClient{},
				users: &staticUserClient{byID: map[string]*userv1.UserResponse{
					"u-client": {Id: "u-client", Name: "Анна Клиент"},
				}},
				threads: []*chatv1.ChatThread{
					{Id: "t1", Kind: "venue_booking", RefId: "booking-1"},
					{Id: "t2", Kind: "venue_booking", RefId: "booking-1"},
				},
				viewer: "owner-1",
				want: map[string]string{
					"t1": "Анна Клиент",
					"t2": "Анна Клиент",
				},
				wantBookCalls:  1,
				wantVenueCalls: 0,
			},
			{
				name: "partner_two_bookings_two_clients_no_GetVenue_for_titles",
				booking: &countingBookingClient{byID: map[string]*bookingv1.BookingResponse{
					"b-a": {Id: "b-a", VenueId: "ven-1", UserId: "c1", VenueName: ""},
					"b-b": {Id: "b-b", VenueId: "ven-1", UserId: "c2", VenueName: ""},
				}},
				venue: &countingVenueClient{byID: map[string]*venuev1.VenueResponse{
					"ven-1": {Id: "ven-1", Name: "Общая сауна"},
				}},
				users: &staticUserClient{byID: map[string]*userv1.UserResponse{
					"c1": {Id: "c1", Name: "Иван"},
					"c2": {Id: "c2", Name: "Мария"},
				}},
				threads: []*chatv1.ChatThread{
					{Id: "t-a", Kind: "venue_booking", RefId: "b-a"},
					{Id: "t-b", Kind: "venue_booking", RefId: "b-b"},
				},
				viewer: "owner",
				want: map[string]string{
					"t-a": "Иван",
					"t-b": "Мария",
				},
				wantBookCalls:  2,
				wantVenueCalls: 0,
			},
			{
				name: "guest_sees_venue_name_fallback_GetVenue_once",
				booking: &countingBookingClient{byID: map[string]*bookingv1.BookingResponse{
					"b-guest": {Id: "b-guest", VenueId: "ven-x", UserId: "guest-u", VenueName: ""},
				}},
				venue: &countingVenueClient{byID: map[string]*venuev1.VenueResponse{
					"ven-x": {Id: "ven-x", Name: "Сауна у озера"},
				}},
				users: &staticUserClient{},
				threads: []*chatv1.ChatThread{
					{Id: "tg", Kind: "venue_booking", RefId: "b-guest"},
				},
				viewer: "guest-u",
				want: map[string]string{
					"tg": "Сауна у озера",
				},
				wantBookCalls:  1,
				wantVenueCalls: 1,
			},
		}

		for _, tc := range venueCases {
			t.Run(tc.name, func(t *testing.T) {
				h := NewChatHandler(nil, tc.booking, tc.venue, &countingMasterClient{}, tc.users, nil, nil, nil)
				got := h.peerDisplayNamesBatch(ctx, tc.viewer, tc.threads)
				for id, w := range tc.want {
					if got[id] != w {
						t.Fatalf("peer[%s]=%q want %q (full %v)", id, got[id], w, got)
					}
				}
				if tc.booking.getCalls != tc.wantBookCalls {
					t.Fatalf("GetBooking calls=%d want %d", tc.booking.getCalls, tc.wantBookCalls)
				}
				if tc.venue.getCalls != tc.wantVenueCalls {
					t.Fatalf("GetVenue calls=%d want %d", tc.venue.getCalls, tc.wantVenueCalls)
				}
			})
		}
	})

	t.Run("master_booking", func(t *testing.T) {
		mb := &countingMasterClient{byRef: map[string]*masterv1.MasterBooking{
			"m1": {
				Id:           "m1",
				ClientUserId: "client-1",
				MasterUserId: proto.String("master-owner-1"),
			},
		}}
		users := &staticUserClient{byID: map[string]*userv1.UserResponse{
			"master-owner-1": {Id: "master-owner-1", Name: "Иван Пармастер"},
			"client-1":       {Id: "client-1", Name: "Пётр Клиент"},
		}}
		h := NewChatHandler(nil, &countingBookingClient{}, &countingVenueClient{}, mb, users, nil, nil, nil)
		threads := []*chatv1.ChatThread{
			{Id: "th1", Kind: "master_booking", RefId: "m1"},
		}

		masterCases := []struct {
			name   string
			viewer string
			want   string
		}{
			{name: "client_sees_master_name", viewer: "client-1", want: "Иван Пармастер"},
			{name: "master_sees_client_name", viewer: "master-owner-1", want: "Пётр Клиент"},
		}

		for i, mc := range masterCases {
			t.Run(mc.name, func(t *testing.T) {
				mb.mu.Lock()
				start := mb.getCalls
				mb.mu.Unlock()

				got := h.peerDisplayNamesBatch(ctx, mc.viewer, threads)
				if got["th1"] != mc.want {
					t.Fatalf("got %q want %q", got["th1"], mc.want)
				}

				mb.mu.Lock()
				delta := mb.getCalls - start
				mb.mu.Unlock()
				if delta != 1 {
					t.Fatalf("GetMasterBooking calls in this batch=%d want 1 (iteration %d)", delta, i)
				}
			})
		}
	})
}
