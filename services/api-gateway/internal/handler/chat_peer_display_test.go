package handler

import (
	"context"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type countingBookingClient struct {
	mu         sync.Mutex
	batchCalls int
	byID       map[string]*bookingv1.BookingResponse
}

// GetBooking is kept for completeness but peerDisplayNamesBatch no longer calls it.
func (c *countingBookingClient) GetBooking(_ context.Context, in *bookingv1.GetBookingRequest, _ ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	if c.byID == nil {
		return nil, nil
	}
	return c.byID[in.GetId()], nil
}

func (c *countingBookingClient) GetBookingsBatch(_ context.Context, in *bookingv1.GetBookingsBatchRequest, _ ...grpc.CallOption) (*bookingv1.GetBookingsBatchResponse, error) {
	c.mu.Lock()
	c.batchCalls++
	c.mu.Unlock()
	resp := &bookingv1.GetBookingsBatchResponse{
		Bookings: make(map[string]*bookingv1.BookingResponse),
	}
	for _, id := range in.GetIds() {
		if b := c.byID[id]; b != nil {
			resp.Bookings[id] = b
		}
	}
	return resp, nil
}

type countingVenueClient struct {
	mu         sync.Mutex
	batchCalls int
	byID       map[string]*venuev1.VenueResponse
}

// GetVenue is kept for completeness but peerDisplayNamesBatch no longer calls it.
func (c *countingVenueClient) GetVenue(_ context.Context, in *venuev1.GetVenueRequest, _ ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if c.byID == nil {
		return nil, nil
	}
	return c.byID[in.GetId()], nil
}

func (c *countingVenueClient) GetVenuesBatch(_ context.Context, in *venuev1.GetVenuesBatchRequest, _ ...grpc.CallOption) (*venuev1.GetVenuesBatchResponse, error) {
	c.mu.Lock()
	c.batchCalls++
	c.mu.Unlock()
	resp := &venuev1.GetVenuesBatchResponse{
		Venues: make(map[string]*venuev1.VenueResponse),
	}
	for _, id := range in.GetIds() {
		if v := c.byID[id]; v != nil {
			resp.Venues[id] = v
		}
	}
	return resp, nil
}

// CRM methods removed: venueGatewayClient no longer requires them.

type countingMasterClient struct {
	mu         sync.Mutex
	batchCalls int
	byRef      map[string]*masterv1.MasterBooking
}

// GetMasterBooking is kept for completeness but peerDisplayNamesBatch no longer calls it.
func (c *countingMasterClient) GetMasterBooking(_ context.Context, in *masterv1.GetMasterBookingRequest, _ ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	bk := c.byRef[in.GetBookingId()]
	if bk == nil {
		return &masterv1.MasterBookingResponse{}, nil
	}
	return &masterv1.MasterBookingResponse{Booking: bk}, nil
}

func (c *countingMasterClient) GetMasterBookingsBatch(_ context.Context, in *masterv1.GetMasterBookingsBatchRequest, _ ...grpc.CallOption) (*masterv1.GetMasterBookingsBatchResponse, error) {
	c.mu.Lock()
	c.batchCalls++
	c.mu.Unlock()
	resp := &masterv1.GetMasterBookingsBatchResponse{
		Bookings: make(map[string]*masterv1.MasterBooking),
	}
	for _, id := range in.GetBookingIds() {
		if bk := c.byRef[id]; bk != nil {
			resp.Bookings[id] = bk
		}
	}
	return resp, nil
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

func (c *staticUserClient) GetUsersBatch(_ context.Context, in *userv1.GetUsersBatchRequest, _ ...grpc.CallOption) (*userv1.GetUsersBatchResponse, error) {
	resp := &userv1.GetUsersBatchResponse{
		Users: make(map[string]*userv1.UserResponse),
	}
	if c == nil || c.byID == nil {
		return resp, nil
	}
	for _, id := range in.GetIds() {
		if u := c.byID[id]; u != nil {
			resp.Users[id] = u
		}
	}
	return resp, nil
}

// TestPeerDisplayNamesBatch — table-driven сценарии для резолва peer_display_name (бывший peerDisplayNameForThread).
func TestPeerDisplayNamesBatch(t *testing.T) {
	ctx := context.Background()

	t.Run("venue_booking", func(t *testing.T) {
		venueCases := []struct {
			name                string
			threads             []*chatv1.ChatThread
			booking             *countingBookingClient
			venue               *countingVenueClient
			users               *staticUserClient
			viewer              string
			want                map[string]string
			wantBookBatchCalls  int
			wantVenueBatchCalls int
		}{
			{
				// Two threads reference the same booking → single GetBookingsBatch call.
				name: "partner_sees_client_name_same_booking_twice_one_batch",
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
				wantBookBatchCalls:  1,
				wantVenueBatchCalls: 0,
			},
			{
				// Two different bookings for the owner → one GetBookingsBatch, no GetVenuesBatch
				// (viewer is owner, so venue name not needed).
				name: "partner_two_bookings_two_clients_one_batch_no_venue_batch",
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
				wantBookBatchCalls:  1,
				wantVenueBatchCalls: 0,
			},
			{
				// Guest, venue_name absent → one GetBookingsBatch + one GetVenuesBatch.
				name: "guest_sees_venue_name_fallback_one_venue_batch",
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
				wantBookBatchCalls:  1,
				wantVenueBatchCalls: 1,
			},
		}

		for _, tc := range venueCases {
			t.Run(tc.name, func(t *testing.T) {
				h := NewChatHandler(context.Background(), zerolog.Nop(), nil, tc.booking, tc.venue, &countingMasterClient{}, tc.users, &resolverFakeCRM{}, nil, nil, nil)
				got := h.resolver.peerDisplayNamesBatch(ctx, tc.viewer, tc.threads)
				for id, w := range tc.want {
					if got[id] != w {
						t.Fatalf("peer[%s]=%q want %q (full %v)", id, got[id], w, got)
					}
				}
				if tc.booking.batchCalls != tc.wantBookBatchCalls {
					t.Fatalf("GetBookingsBatch calls=%d want %d", tc.booking.batchCalls, tc.wantBookBatchCalls)
				}
				if tc.venue.batchCalls != tc.wantVenueBatchCalls {
					t.Fatalf("GetVenuesBatch calls=%d want %d", tc.venue.batchCalls, tc.wantVenueBatchCalls)
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
		h := NewChatHandler(context.Background(), zerolog.Nop(), nil, &countingBookingClient{}, &countingVenueClient{}, mb, users, &resolverFakeCRM{}, nil, nil, nil)
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
				start := mb.batchCalls
				mb.mu.Unlock()

				got := h.resolver.peerDisplayNamesBatch(ctx, mc.viewer, threads)
				if got["th1"] != mc.want {
					t.Fatalf("got %q want %q", got["th1"], mc.want)
				}

				mb.mu.Lock()
				delta := mb.batchCalls - start
				mb.mu.Unlock()
				if delta != 1 {
					t.Fatalf("GetMasterBookingsBatch calls in this batch=%d want 1 (iteration %d)", delta, i)
				}
			})
		}
	})
}
