package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
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

func (f *fakeBookingClient) GetBooking(context.Context, *bookingv1.GetBookingRequest, ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	return f.booking, nil
}

type fakeVenueClient struct {
	ownerID string
	access  string
	staff   []string
}

func (f *fakeVenueClient) GetVenue(context.Context, *venuev1.GetVenueRequest, ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	return &venuev1.VenueResponse{OwnerId: f.ownerID}, nil
}
func (f *fakeVenueClient) GetVenueManagementAccess(context.Context, *venuev1.GetVenueManagementAccessRequest, ...grpc.CallOption) (*venuev1.GetVenueManagementAccessResponse, error) {
	return &venuev1.GetVenueManagementAccessResponse{Access: f.access}, nil
}
func (f *fakeVenueClient) ListVenueStaff(context.Context, *venuev1.ListVenueStaffRequest, ...grpc.CallOption) (*venuev1.ListVenueStaffResponse, error) {
	out := make([]*venuev1.VenueStaffMember, 0, len(f.staff))
	for _, id := range f.staff {
		out = append(out, &venuev1.VenueStaffMember{UserId: id, Role: "staff"})
	}
	return &venuev1.ListVenueStaffResponse{Members: out}, nil
}

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
			h := NewChatHandler(&fakeChatClient{}, &fakeBookingClient{booking: tc.booking}, tc.venue, tc.master, nil, nil, nil, nil)
			body := []byte(`{"kind":"` + tc.kind + `","ref_id":"` + tc.refID + `"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/threads", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.CtxUserID, tc.userID))
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
	h := NewChatHandler(&fakeChatClientSendCapture{}, &fakeBookingClient{}, &fakeVenueClient{}, &fakeMasterClient{}, nil, nil, nil, nil)
	body := []byte(`{"text":"hi","client_msg_id":"c-123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/v2/chat/threads/t1/messages", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxUserID, "u1"))
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
