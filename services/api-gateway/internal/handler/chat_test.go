package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type fakeChatClient struct{}

var lastSendReq *chatv1.SendMessageRequest

func (f *fakeChatClient) EnsureThread(ctx context.Context, in *chatv1.EnsureThreadRequest, opts ...grpc.CallOption) (*chatv1.ThreadResponse, error) {
	return &chatv1.ThreadResponse{Thread: &chatv1.ChatThread{
		Id:                 "thread-1",
		Kind:               in.GetKind(),
		RefId:              in.GetRefId(),
		ParticipantUserIds: in.GetParticipantUserIds(),
	}}, nil
}
func (f *fakeChatClient) ListThreads(context.Context, *chatv1.ListThreadsRequest, ...grpc.CallOption) (*chatv1.ListThreadsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}
func (f *fakeChatClient) ListMessages(context.Context, *chatv1.ListMessagesRequest, ...grpc.CallOption) (*chatv1.ListMessagesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}
func (f *fakeChatClient) SendMessage(context.Context, *chatv1.SendMessageRequest, ...grpc.CallOption) (*chatv1.MessageResponse, error) {
	// keep request for assertions in tests
	lastSendReq = nil
	return nil, status.Error(codes.Unimplemented, "unused")
}
func (f *fakeChatClient) MarkRead(context.Context, *chatv1.MarkReadRequest, ...grpc.CallOption) (*chatv1.ThreadResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}

type fakeBookingClient struct {
	booking *bookingv1.BookingResponse
}

func (f *fakeBookingClient) GetBooking(_ context.Context, in *bookingv1.GetBookingRequest, _ ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	return f.booking, nil
}

func (f *fakeBookingClient) GetBookingsBatch(_ context.Context, in *bookingv1.GetBookingsBatchRequest, _ ...grpc.CallOption) (*bookingv1.GetBookingsBatchResponse, error) {
	resp := &bookingv1.GetBookingsBatchResponse{Bookings: make(map[string]*bookingv1.BookingResponse)}
	if f.booking != nil {
		for _, id := range in.GetIds() {
			if id == f.booking.GetId() {
				resp.Bookings[id] = f.booking
			}
		}
	}
	return resp, nil
}

type fakeVenueClient struct {
	ownerID string
	access  string
	staff   []string
}

func (f *fakeVenueClient) GetVenue(context.Context, *venuev1.GetVenueRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return &venuev1.VenueResponse{OwnerId: f.ownerID}, nil
}
func (f *fakeVenueClient) GetVenuesBatch(_ context.Context, in *venuev1.GetVenuesBatchRequest, _ ...grpc.CallOption) (*venuev1.GetVenuesBatchResponse, error) {
	return &venuev1.GetVenuesBatchResponse{Venues: make(map[string]*venuev1.VenueResponse)}, nil
}
// Note: f.access / f.staff drive the resolverFakeCRM mirrored at the
// NewChatHandler call site (see TestChatEnsureThread_ACLMatrix), since
// CRM RPCs no longer live on venue.proto.

type fakeMasterClient struct {
	booking *masterv1.MasterBooking
	err     error
}

func (f *fakeMasterClient) GetMasterBooking(context.Context, *masterv1.GetMasterBookingRequest, ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &masterv1.MasterBookingResponse{Booking: f.booking}, nil
}

func (f *fakeMasterClient) GetMasterBookingsBatch(_ context.Context, in *masterv1.GetMasterBookingsBatchRequest, _ ...grpc.CallOption) (*masterv1.GetMasterBookingsBatchResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	resp := &masterv1.GetMasterBookingsBatchResponse{Bookings: make(map[string]*masterv1.MasterBooking)}
	if f.booking != nil {
		for _, id := range in.GetBookingIds() {
			if id == f.booking.GetId() {
				resp.Bookings[id] = f.booking
			}
		}
	}
	return resp, nil
}

func TestChatEnsureThread_ACLMatrix(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		kind    string
		status  int
		venue   *fakeVenueClient
		booking *bookingv1.BookingResponse
		master  *fakeMasterClient
		refID   string
	}{
		{
			name:   "venue booking client allowed",
			userID: "client-1",
			kind:   "venue_booking",
			status: http.StatusOK,
			venue:  &fakeVenueClient{ownerID: "owner-1", access: "", staff: []string{"staff-1"}},
			booking: &bookingv1.BookingResponse{
				Id: "b1", VenueId: "v1", UserId: "client-1",
			},
			master: &fakeMasterClient{},
			refID:  "b1",
		},
		{
			name:   "venue booking owner allowed",
			userID: "owner-1",
			kind:   "venue_booking",
			status: http.StatusOK,
			venue:  &fakeVenueClient{ownerID: "owner-1", access: "owner", staff: []string{"staff-1"}},
			booking: &bookingv1.BookingResponse{
				Id: "b1", VenueId: "v1", UserId: "client-1",
			},
			master: &fakeMasterClient{},
			refID:  "b1",
		},
		{
			name:   "venue booking outsider denied",
			userID: "outsider-1",
			kind:   "venue_booking",
			status: http.StatusForbidden,
			venue:  &fakeVenueClient{ownerID: "owner-1", access: "", staff: []string{"staff-1"}},
			booking: &bookingv1.BookingResponse{
				Id: "b1", VenueId: "v1", UserId: "client-1",
			},
			master: &fakeMasterClient{},
			refID:  "b1",
		},
		{
			name:   "master booking client allowed",
			userID: "client-1",
			kind:   "master_booking",
			status: http.StatusOK,
			venue:  &fakeVenueClient{},
			booking: &bookingv1.BookingResponse{
				Id: "x", VenueId: "x", UserId: "x",
			},
			master: &fakeMasterClient{booking: &masterv1.MasterBooking{
				Id:           "mb1",
				ClientUserId: "client-1",
				MasterUserId: proto.String("master-owner-1"),
			}},
			refID: "mb1",
		},
		{
			name:   "master booking master owner allowed",
			userID: "master-owner-1",
			kind:   "master_booking",
			status: http.StatusOK,
			venue:  &fakeVenueClient{},
			booking: &bookingv1.BookingResponse{
				Id: "x", VenueId: "x", UserId: "x",
			},
			master: &fakeMasterClient{booking: &masterv1.MasterBooking{
				Id:           "mb1",
				ClientUserId: "client-1",
				MasterUserId: proto.String("master-owner-1"),
			}},
			refID: "mb1",
		},
		{
			name:   "master booking denied from service",
			userID: "outsider-1",
			kind:   "master_booking",
			status: http.StatusForbidden,
			venue:  &fakeVenueClient{},
			booking: &bookingv1.BookingResponse{
				Id: "x", VenueId: "x", UserId: "x",
			},
			master: &fakeMasterClient{err: status.Error(codes.PermissionDenied, "denied")},
			refID:  "mb1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The CRM extraction moved access / staff out of venue-service.
			// Tests still drive the scenario from fakeVenueClient.access/staff
			// — mirror those into a resolverFakeCRM so the handler observes the
			// same world.
			crm := &resolverFakeCRM{access: tc.venue.access, staff: tc.venue.staff}
			h := NewChatHandler(context.Background(), zerolog.Nop(), &fakeChatClient{}, &fakeBookingClient{booking: tc.booking}, tc.venue, tc.master, nil, crm, nil, nil, nil)
			body := []byte(`{"kind":"` + tc.kind + `","ref_id":"` + tc.refID + `"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/threads", bytes.NewReader(body))
			req = req.WithContext(middleware.WithUserID(req.Context(), tc.userID))
			rec := httptest.NewRecorder()
			h.EnsureThread(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d, want %d, body=%s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

type fakeChatClientSendCapture struct{}

func (f *fakeChatClientSendCapture) EnsureThread(context.Context, *chatv1.EnsureThreadRequest, ...grpc.CallOption) (*chatv1.ThreadResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}
func (f *fakeChatClientSendCapture) ListThreads(context.Context, *chatv1.ListThreadsRequest, ...grpc.CallOption) (*chatv1.ListThreadsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}
func (f *fakeChatClientSendCapture) ListMessages(context.Context, *chatv1.ListMessagesRequest, ...grpc.CallOption) (*chatv1.ListMessagesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}
func (f *fakeChatClientSendCapture) SendMessage(_ context.Context, in *chatv1.SendMessageRequest, _ ...grpc.CallOption) (*chatv1.MessageResponse, error) {
	lastSendReq = in
	return &chatv1.MessageResponse{
		Thread: &chatv1.ChatThread{Id: in.GetThreadId(), ParticipantUserIds: []string{in.GetUserId(), "u2"}},
		Message: &chatv1.ChatMessage{
			Id: "m1", ThreadId: in.GetThreadId(), AuthorUserId: in.GetUserId(), Text: in.GetText(), ClientMsgId: in.GetClientMsgId(),
		},
	}, nil
}
func (f *fakeChatClientSendCapture) MarkRead(context.Context, *chatv1.MarkReadRequest, ...grpc.CallOption) (*chatv1.ThreadResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}

func TestChatSendMessage_PropagatesClientMsgID(t *testing.T) {
	h := NewChatHandler(context.Background(), zerolog.Nop(), &fakeChatClientSendCapture{}, &fakeBookingClient{}, &fakeVenueClient{}, &fakeMasterClient{}, nil, &resolverFakeCRM{}, nil, nil, nil)
	body := []byte(`{"text":"hi","client_msg_id":"c-123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/v2/chat/threads/t1/messages", bytes.NewReader(body))
	req = req.WithContext(middleware.WithUserID(req.Context(), "u1"))
	rec := httptest.NewRecorder()
	// URL param emulation for chi
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("threadId", "t1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	h.SendMessage(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", rec.Code, rec.Body.String())
	}
	if lastSendReq == nil || lastSendReq.GetClientMsgId() != "c-123" {
		t.Fatalf("expected client_msg_id propagated, got %#v", lastSendReq)
	}
}
