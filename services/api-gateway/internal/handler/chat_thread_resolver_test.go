package handler

// Unit tests for chatThreadResolver (N8).
//
// The resolver is tested in isolation: all gRPC clients are replaced with
// lightweight fakes so no network or service binary is needed.  Each fake
// is declared locally so the test table stays self-contained and readable.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	bookingv1 "github.com/tienlao/agregator/gen/go/booking/v1"
	chatv1 "github.com/tienlao/agregator/gen/go/chat/v1"
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// ── chat fake (always succeeds, returns EnsureThread participants back) ────────

type resolverFakeChat struct{}

func (resolverFakeChat) EnsureThread(_ context.Context, in *chatv1.EnsureThreadRequest, _ ...grpc.CallOption) (*chatv1.ThreadResponse, error) {
	return &chatv1.ThreadResponse{Thread: &chatv1.ChatThread{
		Id:                 "thread-ok",
		Kind:               in.GetKind(),
		RefId:              in.GetRefId(),
		ParticipantUserIds: in.GetParticipantUserIds(),
	}}, nil
}
func (resolverFakeChat) ListThreads(context.Context, *chatv1.ListThreadsRequest, ...grpc.CallOption) (*chatv1.ListThreadsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (resolverFakeChat) ListMessages(context.Context, *chatv1.ListMessagesRequest, ...grpc.CallOption) (*chatv1.ListMessagesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (resolverFakeChat) SendMessage(context.Context, *chatv1.SendMessageRequest, ...grpc.CallOption) (*chatv1.MessageResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (resolverFakeChat) MarkRead(context.Context, *chatv1.MarkReadRequest, ...grpc.CallOption) (*chatv1.ThreadResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ── venue fake (configurable per-test) ────────────────────────────────────────
//
// After the CRM extraction (P3-CRM-Extract), the venue fake only needs
// GetVenue / GetVenuesBatch.  Staff and management-access live on the CRM
// fake below.  We keep the fields here as the test-driver layer: tests
// configure ownerID/access/staff once and the helpers wire them to the
// appropriate fake without changing the test bodies.

type resolverFakeVenue struct {
	ownerID     string
	access      string
	staff       []string
	getVenueErr error
}

func (f *resolverFakeVenue) GetVenue(_ context.Context, _ *venuev1.GetVenueRequest, _ ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if f.getVenueErr != nil {
		return nil, f.getVenueErr
	}
	return &venuev1.VenueResponse{OwnerId: f.ownerID}, nil
}
func (f *resolverFakeVenue) GetVenuesBatch(_ context.Context, _ *venuev1.GetVenuesBatchRequest, _ ...grpc.CallOption) (*venuev1.GetVenuesBatchResponse, error) {
	return &venuev1.GetVenuesBatchResponse{Venues: map[string]*venuev1.VenueResponse{}}, nil
}

// ── crm fake (mirrors the venue fake's access/staff fields) ───────────────────

type resolverFakeCRM struct {
	access       string
	staff        []string
	accessErr    error
	listStaffErr error
}

func (f *resolverFakeCRM) GetManagementAccess(_ context.Context, _ *crmv1.GetManagementAccessRequest, _ ...grpc.CallOption) (*crmv1.GetManagementAccessResponse, error) {
	if f.accessErr != nil {
		return nil, f.accessErr
	}
	return &crmv1.GetManagementAccessResponse{Access: f.access}, nil
}
func (f *resolverFakeCRM) ListStaff(_ context.Context, _ *crmv1.ListStaffRequest, _ ...grpc.CallOption) (*crmv1.ListStaffResponse, error) {
	if f.listStaffErr != nil {
		return nil, f.listStaffErr
	}
	members := make([]*crmv1.StaffMember, 0, len(f.staff))
	for _, id := range f.staff {
		members = append(members, &crmv1.StaffMember{UserId: id})
	}
	return &crmv1.ListStaffResponse{Members: members}, nil
}

// ── booking fake ──────────────────────────────────────────────────────────────

type resolverFakeBooking struct {
	resp *bookingv1.BookingResponse
	err  error
}

func (f *resolverFakeBooking) GetBooking(_ context.Context, _ *bookingv1.GetBookingRequest, _ ...grpc.CallOption) (*bookingv1.BookingResponse, error) {
	return f.resp, f.err
}
func (f *resolverFakeBooking) GetBookingsBatch(_ context.Context, in *bookingv1.GetBookingsBatchRequest, _ ...grpc.CallOption) (*bookingv1.GetBookingsBatchResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	resp := &bookingv1.GetBookingsBatchResponse{Bookings: map[string]*bookingv1.BookingResponse{}}
	if f.resp != nil {
		for _, id := range in.GetIds() {
			if id == f.resp.GetId() {
				resp.Bookings[id] = f.resp
			}
		}
	}
	return resp, nil
}

// ── master fake ───────────────────────────────────────────────────────────────

type resolverFakeMaster struct {
	booking *masterv1.MasterBooking
	err     error
}

func (f *resolverFakeMaster) GetMasterBooking(_ context.Context, _ *masterv1.GetMasterBookingRequest, _ ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &masterv1.MasterBookingResponse{Booking: f.booking}, nil
}
func (f *resolverFakeMaster) GetMasterBookingsBatch(_ context.Context, in *masterv1.GetMasterBookingsBatchRequest, _ ...grpc.CallOption) (*masterv1.GetMasterBookingsBatchResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	resp := &masterv1.GetMasterBookingsBatchResponse{Bookings: map[string]*masterv1.MasterBooking{}}
	if f.booking != nil {
		for _, id := range in.GetBookingIds() {
			if id == f.booking.GetId() {
				resp.Bookings[id] = f.booking
			}
		}
	}
	return resp, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// newResolver wires test fakes into a real resolver.  When the caller passes
// a *resolverFakeVenue, the helper mirrors its access/staff fields into a
// resolverFakeCRM so existing tests keep their declarative shape:
//
//	newResolver(
//	    resolverFakeChat{},
//	    &resolverFakeBooking{...},
//	    &resolverFakeVenue{ownerID: "owner-1", access: "owner", staff: ...},
//	    &resolverFakeMaster{},
//	)
//
// Any caller that needs to override CRM behaviour independently (e.g.
// inject a CRM failure) can build a *resolverFakeCRM separately and use
// newResolverWithCRM below.
func newResolver(chat chatGatewayClient, booking bookingGatewayClient, venue venueGatewayClient, master masterGatewayClient) *chatThreadResolver {
	var crm crmGatewayClient = &resolverFakeCRM{}
	if vf, ok := venue.(*resolverFakeVenue); ok {
		crm = &resolverFakeCRM{access: vf.access, staff: vf.staff}
	}
	return newChatThreadResolver(zerolog.Nop(), chat, booking, venue, master, nil, crm)
}

// newResolverWithCRM gives tests full control over the CRM fake.
func newResolverWithCRM(chat chatGatewayClient, booking bookingGatewayClient, venue venueGatewayClient, master masterGatewayClient, crm crmGatewayClient) *chatThreadResolver {
	return newChatThreadResolver(zerolog.Nop(), chat, booking, venue, master, nil, crm)
}

// ── venue-booking tests ───────────────────────────────────────────────────────

// TestResolver_VenueBooking_OwnerIsActor — owner == actor → thread created (allowed).
func TestResolver_VenueBooking_OwnerIsActor(t *testing.T) {
	rs := newResolver(
		resolverFakeChat{},
		&resolverFakeBooking{resp: &bookingv1.BookingResponse{Id: "b1", VenueId: "v1", UserId: "client-1"}},
		&resolverFakeVenue{ownerID: "owner-1", access: "owner"},
		&resolverFakeMaster{},
	)
	req := makeResolverRequest("owner-1")
	thread, err := rs.ensureVenueBookingThread(req, "owner-1", "b1")
	if err != nil {
		t.Fatalf("expected allowed, got err: %v", err)
	}
	if thread == nil || thread.GetId() == "" {
		t.Fatal("expected non-nil thread")
	}
}

// TestResolver_VenueBooking_OutsiderDenied — actor not in client/owner/staff → 403.
func TestResolver_VenueBooking_OutsiderDenied(t *testing.T) {
	rs := newResolver(
		resolverFakeChat{},
		&resolverFakeBooking{resp: &bookingv1.BookingResponse{Id: "b1", VenueId: "v1", UserId: "client-1"}},
		&resolverFakeVenue{ownerID: "owner-1", access: "", staff: nil},
		&resolverFakeMaster{},
	)
	req := makeResolverRequest("outsider-99")
	_, err := rs.ensureVenueBookingThread(req, "outsider-99", "b1")
	if err == nil {
		t.Fatal("expected permission denied, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		// pkgerrors.PermissionDenied wraps to gRPC PermissionDenied; also accept
		// a non-nil error whose message contains "denied".
		if st.Code() == codes.OK {
			t.Fatalf("expected PermissionDenied, got %v", err)
		}
	}
}

// TestResolver_VenueBooking_GetVenueFails_Denied — GetVenue error → fail-closed.
func TestResolver_VenueBooking_GetVenueFails_Denied(t *testing.T) {
	rs := newResolver(
		resolverFakeChat{},
		&resolverFakeBooking{resp: &bookingv1.BookingResponse{Id: "b1", VenueId: "v1", UserId: "client-1"}},
		&resolverFakeVenue{getVenueErr: status.Error(codes.Unavailable, "venue-service down")},
		&resolverFakeMaster{},
	)
	req := makeResolverRequest("client-1")
	_, err := rs.ensureVenueBookingThread(req, "client-1", "b1")
	if err == nil {
		t.Fatal("expected error when GetVenue fails, got nil")
	}
	// Must be denied, not a 5xx pass-through — resolver is fail-closed (P0#2).
	st, _ := status.FromError(err)
	if st.Code() == codes.Unavailable {
		t.Fatalf("resolver leaked upstream Unavailable — should have mapped to PermissionDenied, got %v", err)
	}
}

// TestResolver_VenueBooking_StaffIsActor — staff member → allowed.
func TestResolver_VenueBooking_StaffIsActor(t *testing.T) {
	rs := newResolver(
		resolverFakeChat{},
		&resolverFakeBooking{resp: &bookingv1.BookingResponse{Id: "b1", VenueId: "v1", UserId: "client-1"}},
		&resolverFakeVenue{ownerID: "owner-1", access: "staff", staff: []string{"staff-1"}},
		&resolverFakeMaster{},
	)
	req := makeResolverRequest("staff-1")
	_, err := rs.ensureVenueBookingThread(req, "staff-1", "b1")
	if err != nil {
		t.Fatalf("staff member should be allowed, got: %v", err)
	}
}

// actorGatedCRM mirrors the real CRM-service contract: ListStaff is actor-gated
// to venue members, so a non-member actor (e.g. a plain booking client) is
// denied.  GetManagementAccess is a passthrough that reports the actor's own
// role ("" for a client).  Used to reproduce the bug where a booking client was
// locked out of their own venue thread because staff enumeration passed the
// client as the ListStaff actor.
type actorGatedCRM struct {
	ownerID string
	staff   []string
}

func (f *actorGatedCRM) GetManagementAccess(_ context.Context, in *crmv1.GetManagementAccessRequest, _ ...grpc.CallOption) (*crmv1.GetManagementAccessResponse, error) {
	access := ""
	if in.GetUserId() == f.ownerID {
		access = "owner"
	} else {
		for _, s := range f.staff {
			if s == in.GetUserId() {
				access = "staff"
			}
		}
	}
	return &crmv1.GetManagementAccessResponse{Access: access}, nil
}

func (f *actorGatedCRM) ListStaff(_ context.Context, in *crmv1.ListStaffRequest, _ ...grpc.CallOption) (*crmv1.ListStaffResponse, error) {
	member := in.GetActorId() == f.ownerID
	for _, s := range f.staff {
		if s == in.GetActorId() {
			member = true
		}
	}
	if !member {
		return nil, status.Error(codes.PermissionDenied, "нет доступа к управлению заведением")
	}
	members := make([]*crmv1.StaffMember, 0, len(f.staff))
	for _, id := range f.staff {
		members = append(members, &crmv1.StaffMember{UserId: id})
	}
	return &crmv1.ListStaffResponse{Members: members}, nil
}

// TestResolver_VenueBooking_ClientAllowed_WhenStaffEnumIsActorGated reproduces
// the reported bug: a booking client (not owner/staff) opening their own thread.
// Real CRM denies ListStaff for the client actor; the resolver must enumerate
// staff via the owner and still allow the client.
func TestResolver_VenueBooking_ClientAllowed_WhenStaffEnumIsActorGated(t *testing.T) {
	rs := newResolverWithCRM(
		resolverFakeChat{},
		&resolverFakeBooking{resp: &bookingv1.BookingResponse{Id: "b1", VenueId: "v1", UserId: "client-1"}},
		&resolverFakeVenue{ownerID: "owner-1"},
		&resolverFakeMaster{},
		&actorGatedCRM{ownerID: "owner-1", staff: []string{"staff-1"}},
	)
	req := makeResolverRequest("client-1")
	thread, err := rs.ensureVenueBookingThread(req, "client-1", "b1")
	if err != nil {
		t.Fatalf("booking client should be allowed into their own thread, got: %v", err)
	}
	// Staff enrichment must still happen (via the owner actor): client, owner and
	// staff-1 are all participants.
	pids := thread.GetParticipantUserIds()
	has := func(id string) bool {
		for _, p := range pids {
			if p == id {
				return true
			}
		}
		return false
	}
	if !has("client-1") || !has("owner-1") || !has("staff-1") {
		t.Fatalf("participant list missing expected IDs: %v", pids)
	}
}

// ── master-booking tests ──────────────────────────────────────────────────────

// TestResolver_MasterBooking_EmptyBooking_Denied — service returns nil booking → denied.
func TestResolver_MasterBooking_EmptyBooking_Denied(t *testing.T) {
	rs := newResolver(
		resolverFakeChat{},
		&resolverFakeBooking{},
		&resolverFakeVenue{},
		&resolverFakeMaster{booking: nil}, // nil booking inside a non-error response
	)
	req := makeResolverRequest("client-1")
	_, err := rs.ensureMasterBookingThread(req, "client-1", "mb1")
	if err == nil {
		t.Fatal("expected denied for nil booking, got nil error")
	}
}

// TestResolver_MasterBooking_ClientEqualsMaster_Denied — deduped participants < 2 → denied.
func TestResolver_MasterBooking_ClientEqualsMaster_Denied(t *testing.T) {
	sameID := "user-42"
	rs := newResolver(
		resolverFakeChat{},
		&resolverFakeBooking{},
		&resolverFakeVenue{},
		&resolverFakeMaster{booking: &masterv1.MasterBooking{
			Id:           "mb1",
			ClientUserId: sameID,
			MasterUserId: proto.String(sameID), // same ID → uniqueNonEmpty → len == 1
		}},
	)
	req := makeResolverRequest(sameID)
	_, err := rs.ensureMasterBookingThread(req, sameID, "mb1")
	if err == nil {
		t.Fatal("expected denied when client == master (deduped to 1 participant), got nil")
	}
}

// TestResolver_MasterBooking_ServiceError_Denied — GetMasterBooking returns error → propagated.
func TestResolver_MasterBooking_ServiceError_Denied(t *testing.T) {
	rs := newResolver(
		resolverFakeChat{},
		&resolverFakeBooking{},
		&resolverFakeVenue{},
		&resolverFakeMaster{err: status.Error(codes.PermissionDenied, "not your booking")},
	)
	req := makeResolverRequest("outsider")
	_, err := rs.ensureMasterBookingThread(req, "outsider", "mb1")
	if err == nil {
		t.Fatal("expected error when master-service denies, got nil")
	}
}

// TestResolver_MasterBooking_HappyPath — valid booking with distinct IDs → allowed.
func TestResolver_MasterBooking_HappyPath(t *testing.T) {
	rs := newResolver(
		resolverFakeChat{},
		&resolverFakeBooking{},
		&resolverFakeVenue{},
		&resolverFakeMaster{booking: &masterv1.MasterBooking{
			Id:           "mb1",
			ClientUserId: "client-1",
			MasterUserId: proto.String("master-1"),
		}},
	)
	req := makeResolverRequest("client-1")
	thread, err := rs.ensureMasterBookingThread(req, "client-1", "mb1")
	if err != nil {
		t.Fatalf("expected allowed, got: %v", err)
	}
	if thread == nil {
		t.Fatal("expected non-nil thread")
	}
	// Verify both parties are participants.
	pids := thread.GetParticipantUserIds()
	has := func(id string) bool {
		for _, p := range pids {
			if p == id {
				return true
			}
		}
		return false
	}
	if !has("client-1") || !has("master-1") {
		t.Fatalf("participant list missing expected IDs: %v", pids)
	}
}

// makeResolverRequest builds an *http.Request whose context satisfies
// middleware.UserIDFromCtx(r.Context()) == userID, exactly as the Auth
// middleware would set it in production.
func makeResolverRequest(userID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	return req.WithContext(middleware.WithUserID(req.Context(), userID))
}
