package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// masterStub is a flexible MasterServiceClient mock via interface embedding.
type masterStub struct {
	masterv1.MasterServiceClient
	onListPublic   func(*masterv1.ListPublicMastersRequest) (*masterv1.ListMastersResponse, error)
	onGetPublic    func(*masterv1.GetPublicMasterRequest) (*masterv1.MasterResponse, error)
	onCreate       func(*masterv1.CreateMyProfileRequest) (*masterv1.MasterResponse, error)
	onGetMy        func(*masterv1.GetMyProfileRequest) (*masterv1.MasterResponse, error)
	onUpdate       func(*masterv1.UpdateMyProfileRequest) (*masterv1.MasterResponse, error)
	onSubmit       func(*masterv1.SubmitMasterForReviewRequest) (*masterv1.MasterResponse, error)
	onListBookings func(*masterv1.ListMyMasterBookingsRequest) (*masterv1.ListMasterBookingsResponse, error)
	onListClients  func(*masterv1.ListMyMasterClientsRequest) (*masterv1.ListMyMasterClientsResponse, error)
	onListBlocks   func(*masterv1.ListMasterSlotBlocksRequest) (*masterv1.ListMasterSlotBlocksResponse, error)
	onCreateBlock  func(*masterv1.CreateMasterSlotBlockRequest) (*masterv1.MasterSlotBlockResponse, error)
	onDeleteBlock  func(*masterv1.DeleteMasterSlotBlockRequest) (*masterv1.DeleteMasterSlotBlockResponse, error)
	onCreateBook   func(*masterv1.CreateMasterBookingRequest) (*masterv1.MasterBookingResponse, error)
	onListMod      func(*masterv1.ListForModerationRequest) (*masterv1.ListMastersResponse, error)
	onModerate     func(*masterv1.ModerateMasterRequest) (*masterv1.MasterResponse, error)
	onModHistory   func(*masterv1.ListModerationHistoryRequest) (*masterv1.ListModerationHistoryResponse, error)
}

func (m *masterStub) ListPublicMasters(_ context.Context, in *masterv1.ListPublicMastersRequest, _ ...grpc.CallOption) (*masterv1.ListMastersResponse, error) {
	if m.onListPublic != nil {
		return m.onListPublic(in)
	}
	return &masterv1.ListMastersResponse{}, nil
}
func (m *masterStub) GetPublicMaster(_ context.Context, in *masterv1.GetPublicMasterRequest, _ ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	if m.onGetPublic != nil {
		return m.onGetPublic(in)
	}
	return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
}
func (m *masterStub) CreateMyProfile(_ context.Context, in *masterv1.CreateMyProfileRequest, _ ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	if m.onCreate != nil {
		return m.onCreate(in)
	}
	return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
}
func (m *masterStub) GetMyProfile(_ context.Context, in *masterv1.GetMyProfileRequest, _ ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	if m.onGetMy != nil {
		return m.onGetMy(in)
	}
	return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
}
func (m *masterStub) UpdateMyProfile(_ context.Context, in *masterv1.UpdateMyProfileRequest, _ ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	if m.onUpdate != nil {
		return m.onUpdate(in)
	}
	return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
}
func (m *masterStub) SubmitForReview(_ context.Context, in *masterv1.SubmitMasterForReviewRequest, _ ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	if m.onSubmit != nil {
		return m.onSubmit(in)
	}
	return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1", Status: "pending"}}, nil
}
func (m *masterStub) ListMyMasterBookings(_ context.Context, in *masterv1.ListMyMasterBookingsRequest, _ ...grpc.CallOption) (*masterv1.ListMasterBookingsResponse, error) {
	if m.onListBookings != nil {
		return m.onListBookings(in)
	}
	return &masterv1.ListMasterBookingsResponse{}, nil
}
func (m *masterStub) ListMyMasterClients(_ context.Context, in *masterv1.ListMyMasterClientsRequest, _ ...grpc.CallOption) (*masterv1.ListMyMasterClientsResponse, error) {
	if m.onListClients != nil {
		return m.onListClients(in)
	}
	return &masterv1.ListMyMasterClientsResponse{}, nil
}
func (m *masterStub) ListMasterSlotBlocks(_ context.Context, in *masterv1.ListMasterSlotBlocksRequest, _ ...grpc.CallOption) (*masterv1.ListMasterSlotBlocksResponse, error) {
	if m.onListBlocks != nil {
		return m.onListBlocks(in)
	}
	return &masterv1.ListMasterSlotBlocksResponse{}, nil
}
func (m *masterStub) CreateMasterSlotBlock(_ context.Context, in *masterv1.CreateMasterSlotBlockRequest, _ ...grpc.CallOption) (*masterv1.MasterSlotBlockResponse, error) {
	if m.onCreateBlock != nil {
		return m.onCreateBlock(in)
	}
	return &masterv1.MasterSlotBlockResponse{Block: &masterv1.MasterSlotBlock{Id: "b1", CreatedAt: timestamppb.Now()}}, nil
}
func (m *masterStub) DeleteMasterSlotBlock(_ context.Context, in *masterv1.DeleteMasterSlotBlockRequest, _ ...grpc.CallOption) (*masterv1.DeleteMasterSlotBlockResponse, error) {
	if m.onDeleteBlock != nil {
		return m.onDeleteBlock(in)
	}
	return &masterv1.DeleteMasterSlotBlockResponse{}, nil
}
func (m *masterStub) CreateMasterBooking(_ context.Context, in *masterv1.CreateMasterBookingRequest, _ ...grpc.CallOption) (*masterv1.MasterBookingResponse, error) {
	if m.onCreateBook != nil {
		return m.onCreateBook(in)
	}
	return &masterv1.MasterBookingResponse{Booking: &masterv1.MasterBooking{Id: "bk1", CreatedAt: timestamppb.Now()}}, nil
}
func (m *masterStub) ListForModeration(_ context.Context, in *masterv1.ListForModerationRequest, _ ...grpc.CallOption) (*masterv1.ListMastersResponse, error) {
	if m.onListMod != nil {
		return m.onListMod(in)
	}
	return &masterv1.ListMastersResponse{}, nil
}
func (m *masterStub) ModerateMaster(_ context.Context, in *masterv1.ModerateMasterRequest, _ ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	if m.onModerate != nil {
		return m.onModerate(in)
	}
	return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
}
func (m *masterStub) ListModerationHistory(_ context.Context, in *masterv1.ListModerationHistoryRequest, _ ...grpc.CallOption) (*masterv1.ListModerationHistoryResponse, error) {
	if m.onModHistory != nil {
		return m.onModHistory(in)
	}
	return &masterv1.ListModerationHistoryResponse{}, nil
}

type masterNotifierSpy struct {
	bookingCalls int
	moderCalls   int
	lastMasterID string
	lastAction   string
}

func (s *masterNotifierSpy) NotifyMasterBookingCreated(_ context.Context, _, masterID, _, _, _, _, _ string) {
	s.bookingCalls++
	s.lastMasterID = masterID
}
func (s *masterNotifierSpy) NotifyMasterModerated(_ context.Context, _, _, _, action, _ string) {
	s.moderCalls++
	s.lastAction = action
}

// --- ListPublic ---

func TestMasterListPublic_ParsesFiltersAndStripsPayout(t *testing.T) {
	var gotReq *masterv1.ListPublicMastersRequest
	stub := &masterStub{onListPublic: func(in *masterv1.ListPublicMastersRequest) (*masterv1.ListMastersResponse, error) {
		gotReq = in
		return &masterv1.ListMastersResponse{
			Total:   1,
			Masters: []*masterv1.Master{{Id: "m1", PayoutInn: "123"}},
		}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.ListPublic(rr, reviewReq(http.MethodGet, "/masters?page=2&page_size=10&q=spa&city=Moscow&price_min=50&price_max=200&work_format=onsite", "", nil, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if gotReq.GetLimit() != 10 || gotReq.GetOffset() != 10 {
		t.Fatalf("pagination: limit=%d offset=%d", gotReq.GetLimit(), gotReq.GetOffset())
	}
	if gotReq.GetPriceMinKopecks() != 5000 || gotReq.GetPriceMaxKopecks() != 20000 {
		t.Fatalf("price kopecks: min=%d max=%d", gotReq.GetPriceMinKopecks(), gotReq.GetPriceMaxKopecks())
	}
	if len(gotReq.GetCities()) != 1 || gotReq.GetCities()[0] != "Moscow" {
		t.Fatalf("cities = %v", gotReq.GetCities())
	}
	var body struct {
		Masters []map[string]any `json:"masters"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if _, leaked := body.Masters[0]["payout_inn"]; leaked {
		t.Fatal("payout_inn leaked in public listing")
	}
}

func TestMasterListPublic_Error(t *testing.T) {
	stub := &masterStub{onListPublic: func(*masterv1.ListPublicMastersRequest) (*masterv1.ListMastersResponse, error) {
		return nil, status.Error(codes.Internal, "boom")
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.ListPublic(rr, reviewReq(http.MethodGet, "/masters", "", nil, ""))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestMasterGetPublic_Success(t *testing.T) {
	stub := &masterStub{onGetPublic: func(in *masterv1.GetPublicMasterRequest) (*masterv1.MasterResponse, error) {
		if in.GetSlug() != "ivan" {
			t.Fatalf("slug = %s", in.GetSlug())
		}
		return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1", Slug: "ivan"}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.GetPublic(rr, reviewReq(http.MethodGet, "/masters/ivan", "", map[string]string{"slug": "ivan"}, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- CreateMyProfile / GetMyProfile ---

func TestMasterCreateMyProfile_Unauthorized(t *testing.T) {
	h := NewMasterHandler(&masterStub{}, noopUploader{})
	rr := httptest.NewRecorder()
	h.CreateMyProfile(rr, reviewReq(http.MethodPost, "/profile", `{"display_name":"X"}`, nil, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestMasterCreateMyProfile_Success(t *testing.T) {
	var got *masterv1.CreateMyProfileRequest
	stub := &masterStub{onCreate: func(in *masterv1.CreateMyProfileRequest) (*masterv1.MasterResponse, error) {
		got = in
		return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.CreateMyProfile(rr, reviewReq(http.MethodPost, "/profile", `{"display_name":"  Ivan  "}`, nil, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.GetDisplayName() != "Ivan" || got.GetUserId() != "u1" {
		t.Fatalf("req = %+v", got)
	}
}

func TestMasterGetMyProfile_NotFoundReturnsNull(t *testing.T) {
	stub := &masterStub{onGetMy: func(*masterv1.GetMyProfileRequest) (*masterv1.MasterResponse, error) {
		return nil, status.Error(codes.NotFound, "none")
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.GetMyProfile(rr, reviewReq(http.MethodGet, "/profile", "", nil, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["profile"] != nil {
		t.Fatalf("profile = %v, want nil", body["profile"])
	}
}

func TestMasterGetMyProfile_OtherError(t *testing.T) {
	stub := &masterStub{onGetMy: func(*masterv1.GetMyProfileRequest) (*masterv1.MasterResponse, error) {
		return nil, status.Error(codes.Internal, "boom")
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.GetMyProfile(rr, reviewReq(http.MethodGet, "/profile", "", nil, "u1"))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestMasterGetMyProfile_Success(t *testing.T) {
	stub := &masterStub{onGetMy: func(*masterv1.GetMyProfileRequest) (*masterv1.MasterResponse, error) {
		return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1", DisplayName: "Ivan"}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.GetMyProfile(rr, reviewReq(http.MethodGet, "/profile", "", nil, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Profile map[string]any `json:"profile"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Profile["display_name"] != "Ivan" {
		t.Fatalf("profile = %v", body.Profile)
	}
}

// --- PatchMyProfile ---

func TestMasterPatchMyProfile_InvalidServices(t *testing.T) {
	h := NewMasterHandler(&masterStub{}, noopUploader{})
	rr := httptest.NewRecorder()
	// services must be an array; a string triggers updateReqFromRaw error.
	h.PatchMyProfile(rr, reviewReq(http.MethodPatch, "/profile", `{"services":"nope"}`, nil, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayMasterInvalidServices.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestMasterPatchMyProfile_Success(t *testing.T) {
	var got *masterv1.UpdateMyProfileRequest
	stub := &masterStub{onUpdate: func(in *masterv1.UpdateMyProfileRequest) (*masterv1.MasterResponse, error) {
		got = in
		return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.PatchMyProfile(rr, reviewReq(http.MethodPatch, "/profile", `{"display_name":"New","hourly_rate":1500}`, nil, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.GetDisplayName() != "New" || got.GetHourlyRate() != 1500 {
		t.Fatalf("update req = %+v", got)
	}
}

func TestMasterPatchMyProfile_Unauthorized(t *testing.T) {
	h := NewMasterHandler(&masterStub{}, noopUploader{})
	rr := httptest.NewRecorder()
	h.PatchMyProfile(rr, reviewReq(http.MethodPatch, "/profile", `{}`, nil, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- SubmitForReview ---

func TestMasterSubmitForReview_EmptyBody(t *testing.T) {
	updated := false
	stub := &masterStub{
		onUpdate: func(*masterv1.UpdateMyProfileRequest) (*masterv1.MasterResponse, error) {
			updated = true
			return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
		},
		onSubmit: func(*masterv1.SubmitMasterForReviewRequest) (*masterv1.MasterResponse, error) {
			return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1", Status: "pending"}}, nil
		},
	}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.SubmitForReview(rr, reviewReq(http.MethodPost, "/submit", "", nil, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if updated {
		t.Fatal("empty body should not trigger update")
	}
}

func TestMasterSubmitForReview_WithBodyUpdatesFirst(t *testing.T) {
	order := []string{}
	stub := &masterStub{
		onUpdate: func(*masterv1.UpdateMyProfileRequest) (*masterv1.MasterResponse, error) {
			order = append(order, "update")
			return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1"}}, nil
		},
		onSubmit: func(*masterv1.SubmitMasterForReviewRequest) (*masterv1.MasterResponse, error) {
			order = append(order, "submit")
			return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1", Status: "pending"}}, nil
		},
	}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.SubmitForReview(rr, reviewReq(http.MethodPost, "/submit", `{"bio":"hello"}`, nil, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if len(order) != 2 || order[0] != "update" || order[1] != "submit" {
		t.Fatalf("order = %v", order)
	}
}

func TestMasterSubmitForReview_InvalidJSON(t *testing.T) {
	h := NewMasterHandler(&masterStub{}, noopUploader{})
	rr := httptest.NewRecorder()
	h.SubmitForReview(rr, reviewReq(http.MethodPost, "/submit", `{bad`, nil, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestInvalidJson.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

// --- ListMyBookings enriches client name ---

func TestMasterListMyBookings_EnrichesClientName(t *testing.T) {
	stub := &masterStub{onListBookings: func(*masterv1.ListMyMasterBookingsRequest) (*masterv1.ListMasterBookingsResponse, error) {
		return &masterv1.ListMasterBookingsResponse{Bookings: []*masterv1.MasterBooking{
			{Id: "bk1", ClientUserId: "c1", CreatedAt: timestamppb.Now()},
		}}, nil
	}}
	user := &reviewUserMock{onGet: func(id string) (*userv1.UserResponse, error) {
		return &userv1.UserResponse{Id: id, Name: "Client One"}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{}, WithMasterUserClient(user))
	rr := httptest.NewRecorder()
	h.ListMyBookings(rr, reviewReq(http.MethodGet, "/bookings", "", nil, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Bookings []map[string]any `json:"bookings"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Bookings[0]["client_name"] != "Client One" {
		t.Fatalf("client_name = %v", body.Bookings[0]["client_name"])
	}
}

func TestMasterListMyClients_Success(t *testing.T) {
	stub := &masterStub{onListClients: func(*masterv1.ListMyMasterClientsRequest) (*masterv1.ListMyMasterClientsResponse, error) {
		return &masterv1.ListMyMasterClientsResponse{
			Total:   1,
			Clients: []*masterv1.MasterClient{{UserId: "c1", BookingsCount: 3, FirstVisitAt: timestamppb.Now()}},
		}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.ListMyClients(rr, reviewReq(http.MethodGet, "/clients?limit=10&offset=0", "", nil, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- slot blocks ---

func TestMasterSlotBlocks_CRUD(t *testing.T) {
	stub := &masterStub{
		onListBlocks: func(*masterv1.ListMasterSlotBlocksRequest) (*masterv1.ListMasterSlotBlocksResponse, error) {
			return &masterv1.ListMasterSlotBlocksResponse{Blocks: []*masterv1.MasterSlotBlock{{Id: "b1", CreatedAt: timestamppb.Now()}}}, nil
		},
	}
	h := NewMasterHandler(stub, noopUploader{})

	t.Run("list", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ListSlotBlocks(rr, reviewReq(http.MethodGet, "/blocks", "", nil, "u1"))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d", rr.Code)
		}
	})
	t.Run("create", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.CreateSlotBlock(rr, reviewReq(http.MethodPost, "/blocks", `{"date":"2026-01-02","time_from":"10:00","time_to":"12:00"}`, nil, "u1"))
		if rr.Code != http.StatusCreated {
			t.Fatalf("status = %d", rr.Code)
		}
	})
	t.Run("delete", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.DeleteSlotBlock(rr, reviewReq(http.MethodDelete, "/blocks/b1", "", map[string]string{"blockId": "b1"}, "u1"))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d", rr.Code)
		}
	})
	t.Run("create_unauthorized", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.CreateSlotBlock(rr, reviewReq(http.MethodPost, "/blocks", `{}`, nil, ""))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rr.Code)
		}
	})
}

// --- CreateBooking fires notifier ---

func TestMasterCreateBooking_FiresNotifier(t *testing.T) {
	spy := &masterNotifierSpy{}
	stub := &masterStub{onCreateBook: func(in *masterv1.CreateMasterBookingRequest) (*masterv1.MasterBookingResponse, error) {
		if in.GetMasterServiceId() != "svc1" {
			t.Fatalf("service id = %s", in.GetMasterServiceId())
		}
		return &masterv1.MasterBookingResponse{Booking: &masterv1.MasterBooking{
			Id: "bk1", MasterId: "m1", MasterUserId: strptr("master-u"), CreatedAt: timestamppb.Now(),
		}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{}, WithMasterOwnerNotifier(spy))
	rr := httptest.NewRecorder()
	body := `{"master_service_id":"svc1","date":"2026-01-02","time_from":"10:00","time_to":"11:00"}`
	h.CreateBooking(rr, reviewReq(http.MethodPost, "/masters/ivan/bookings", body, map[string]string{"slug": "ivan"}, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if spy.bookingCalls != 1 || spy.lastMasterID != "m1" {
		t.Fatalf("notifier: calls=%d masterID=%s", spy.bookingCalls, spy.lastMasterID)
	}
}

func TestMasterCreateBooking_SelfBookingNoNotify(t *testing.T) {
	spy := &masterNotifierSpy{}
	stub := &masterStub{onCreateBook: func(*masterv1.CreateMasterBookingRequest) (*masterv1.MasterBookingResponse, error) {
		return &masterv1.MasterBookingResponse{Booking: &masterv1.MasterBooking{
			Id: "bk1", MasterUserId: strptr("u1"), CreatedAt: timestamppb.Now(), // == caller
		}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{}, WithMasterOwnerNotifier(spy))
	rr := httptest.NewRecorder()
	h.CreateBooking(rr, reviewReq(http.MethodPost, "/masters/ivan/bookings", `{"date":"2026-01-02","time_from":"10:00","time_to":"11:00"}`, map[string]string{"slug": "ivan"}, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if spy.bookingCalls != 0 {
		t.Fatalf("expected no notification for self-booking, got %d", spy.bookingCalls)
	}
}

// --- admin moderation ---

func TestMasterListForModeration_Success(t *testing.T) {
	stub := &masterStub{onListMod: func(in *masterv1.ListForModerationRequest) (*masterv1.ListMastersResponse, error) {
		if in.GetStatusFilter() != "pending" {
			t.Fatalf("status filter = %s", in.GetStatusFilter())
		}
		return &masterv1.ListMastersResponse{Total: 1, Masters: []*masterv1.Master{{Id: "m1"}}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.ListForModeration(rr, reviewReq(http.MethodGet, "/admin/masters?status=pending", "", nil, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestMasterModerate_FiresNotifier(t *testing.T) {
	spy := &masterNotifierSpy{}
	stub := &masterStub{onModerate: func(in *masterv1.ModerateMasterRequest) (*masterv1.MasterResponse, error) {
		if in.GetAction() != "approve" || in.GetModeratorId() != "admin1" {
			t.Fatalf("moderate req = %+v", in)
		}
		return &masterv1.MasterResponse{Master: &masterv1.Master{Id: "m1", UserId: "master-u", DisplayName: "Ivan"}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{}, WithMasterOwnerNotifier(spy))
	rr := httptest.NewRecorder()
	h.Moderate(rr, reviewReq(http.MethodPost, "/admin/masters/m1/moderate", `{"action":"approve","comment":"ok"}`, map[string]string{"id": "m1"}, "admin1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if spy.moderCalls != 1 || spy.lastAction != "approve" {
		t.Fatalf("notifier: calls=%d action=%s", spy.moderCalls, spy.lastAction)
	}
}

func TestMasterModerate_Unauthorized(t *testing.T) {
	h := NewMasterHandler(&masterStub{}, noopUploader{})
	rr := httptest.NewRecorder()
	h.Moderate(rr, reviewReq(http.MethodPost, "/admin/masters/m1/moderate", `{"action":"approve"}`, map[string]string{"id": "m1"}, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestMasterModerationHistory_Success(t *testing.T) {
	stub := &masterStub{onModHistory: func(*masterv1.ListModerationHistoryRequest) (*masterv1.ListModerationHistoryResponse, error) {
		return &masterv1.ListModerationHistoryResponse{Entries: []*masterv1.ModerationHistoryEntry{
			{Id: "e1", MasterId: "m1", OldStatus: "draft", NewStatus: "pending", CreatedAt: timestamppb.New(time.Now())},
		}}, nil
	}}
	h := NewMasterHandler(stub, noopUploader{})
	rr := httptest.NewRecorder()
	h.ModerationHistory(rr, reviewReq(http.MethodGet, "/admin/masters/m1/moderation-history", "", map[string]string{"id": "m1"}, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Entries []map[string]any `json:"entries"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Entries) != 1 || body.Entries[0]["new_status"] != "pending" {
		t.Fatalf("entries = %v", body.Entries)
	}
}

// --- updateReqFromRaw pure branches ---

func TestUpdateReqFromRaw_AllFieldKinds(t *testing.T) {
	h := &MasterHandler{}
	raw := map[string]json.RawMessage{
		"display_name":         json.RawMessage(`"Ivan"`),
		"travel_radius_km":     json.RawMessage(`15`),
		"hourly_rate":          json.RawMessage(`2000`),
		"travel_base_latitude": json.RawMessage(`55.75`),
		"specializations":      json.RawMessage(`["massage","spa"]`),
	}
	req, err := h.updateReqFromRaw("u1", raw)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if req.GetDisplayName() != "Ivan" || req.GetTravelRadiusKm() != 15 || req.GetHourlyRate() != 2000 {
		t.Fatalf("scalar fields = %+v", req)
	}
	if req.GetTravelBaseLatitude() != 55.75 {
		t.Fatalf("lat = %v", req.GetTravelBaseLatitude())
	}
	if !req.GetApplySpecializations() || len(req.GetSpecializations()) != 2 {
		t.Fatalf("specializations = %v", req.GetSpecializations())
	}
}

func TestUpdateReqFromRaw_ServicesEmptyNotApplied(t *testing.T) {
	h := &MasterHandler{}
	req, err := h.updateReqFromRaw("u1", map[string]json.RawMessage{"services": json.RawMessage(`[]`)})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if req.GetApplyServicesReplace() {
		t.Fatal("empty services array must not trigger replace")
	}
}

func TestUpdateReqFromRaw_ServicesApplied(t *testing.T) {
	h := &MasterHandler{}
	raw := map[string]json.RawMessage{"services": json.RawMessage(`[{"id":"s1","name":"Massage","duration_min":60,"price":1000,"sort_order":1}]`)}
	req, err := h.updateReqFromRaw("u1", raw)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !req.GetApplyServicesReplace() || len(req.GetServicesReplace()) != 1 {
		t.Fatalf("services = %v", req.GetServicesReplace())
	}
	if req.GetServicesReplace()[0].GetId() != "s1" || req.GetServicesReplace()[0].GetName() != "Massage" {
		t.Fatalf("service item = %+v", req.GetServicesReplace()[0])
	}
}

func TestUpdateReqFromRaw_CredentialsEmptyApplied(t *testing.T) {
	h := &MasterHandler{}
	req, err := h.updateReqFromRaw("u1", map[string]json.RawMessage{"credentials": json.RawMessage(`[]`)})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !req.GetApplyCredentials() {
		t.Fatal("credentials key presence must set apply_credentials even when empty")
	}
}

func TestUpdateReqFromRaw_TravelExcludeZones(t *testing.T) {
	h := &MasterHandler{}
	raw := map[string]json.RawMessage{"travel_exclude_zones": json.RawMessage(`[{"id":"z1","latitude":1.0,"longitude":2.0,"radius_km":3.0,"label":"home"}]`)}
	req, err := h.updateReqFromRaw("u1", raw)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !req.GetApplyTravelExcludeZones() || len(req.GetTravelExcludeZones()) != 1 {
		t.Fatalf("zones = %v", req.GetTravelExcludeZones())
	}
}

func TestUpdateReqFromRaw_Errors(t *testing.T) {
	h := &MasterHandler{}
	for _, tc := range []struct {
		name string
		raw  map[string]json.RawMessage
	}{
		{"bad services", map[string]json.RawMessage{"services": json.RawMessage(`"x"`)}},
		{"bad credentials", map[string]json.RawMessage{"credentials": json.RawMessage(`5`)}},
		{"bad zones", map[string]json.RawMessage{"travel_exclude_zones": json.RawMessage(`"x"`)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.updateReqFromRaw("u1", tc.raw); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestSlotBlockToJSON_Nil(t *testing.T) {
	if slotBlockToJSON(nil) != nil {
		t.Fatal("expected nil")
	}
}
