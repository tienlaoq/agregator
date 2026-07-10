package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// crmStub is a flexible CRMServiceClient mock (interface embedding: only the
// exercised methods are wired; the rest panic if unexpectedly called).
type crmStub struct {
	crmv1.CRMServiceClient
	onListStaff func(*crmv1.ListStaffRequest) (*crmv1.ListStaffResponse, error)
	onAddStaff  func(*crmv1.AddStaffRequest) (*crmv1.AddStaffResponse, error)
	onRemove    func(*crmv1.RemoveStaffRequest) (*crmv1.RemoveStaffResponse, error)
	onListTasks func(*crmv1.ListTasksRequest) (*crmv1.ListTasksResponse, error)
	onCreate    func(*crmv1.CreateTaskRequest) (*crmv1.CreateTaskResponse, error)
	onComplete  func(*crmv1.CompleteTaskRequest) (*crmv1.CompleteTaskResponse, error)
	onUpdate    func(*crmv1.UpdateTaskRequest) (*crmv1.UpdateTaskResponse, error)
	onReopen    func(*crmv1.ReopenTaskRequest) (*crmv1.ReopenTaskResponse, error)
	onCancel    func(*crmv1.CancelTaskRequest) (*crmv1.CancelTaskResponse, error)
	onListGuest func(*crmv1.ListGuestsRequest) (*crmv1.ListGuestsResponse, error)
	onGetGuest  func(*crmv1.GetGuestRequest) (*crmv1.GetGuestResponse, error)
}

func (c *crmStub) ListStaff(_ context.Context, in *crmv1.ListStaffRequest, _ ...grpc.CallOption) (*crmv1.ListStaffResponse, error) {
	if c.onListStaff != nil {
		return c.onListStaff(in)
	}
	return &crmv1.ListStaffResponse{}, nil
}
func (c *crmStub) AddStaff(_ context.Context, in *crmv1.AddStaffRequest, _ ...grpc.CallOption) (*crmv1.AddStaffResponse, error) {
	if c.onAddStaff != nil {
		return c.onAddStaff(in)
	}
	return &crmv1.AddStaffResponse{}, nil
}
func (c *crmStub) RemoveStaff(_ context.Context, in *crmv1.RemoveStaffRequest, _ ...grpc.CallOption) (*crmv1.RemoveStaffResponse, error) {
	if c.onRemove != nil {
		return c.onRemove(in)
	}
	return &crmv1.RemoveStaffResponse{}, nil
}
func (c *crmStub) ListTasks(_ context.Context, in *crmv1.ListTasksRequest, _ ...grpc.CallOption) (*crmv1.ListTasksResponse, error) {
	if c.onListTasks != nil {
		return c.onListTasks(in)
	}
	return &crmv1.ListTasksResponse{}, nil
}
func (c *crmStub) CreateTask(_ context.Context, in *crmv1.CreateTaskRequest, _ ...grpc.CallOption) (*crmv1.CreateTaskResponse, error) {
	if c.onCreate != nil {
		return c.onCreate(in)
	}
	return &crmv1.CreateTaskResponse{Task: &crmv1.Task{Id: "t1"}}, nil
}
func (c *crmStub) CompleteTask(_ context.Context, in *crmv1.CompleteTaskRequest, _ ...grpc.CallOption) (*crmv1.CompleteTaskResponse, error) {
	if c.onComplete != nil {
		return c.onComplete(in)
	}
	return &crmv1.CompleteTaskResponse{}, nil
}
func (c *crmStub) UpdateTask(_ context.Context, in *crmv1.UpdateTaskRequest, _ ...grpc.CallOption) (*crmv1.UpdateTaskResponse, error) {
	if c.onUpdate != nil {
		return c.onUpdate(in)
	}
	return &crmv1.UpdateTaskResponse{Task: &crmv1.Task{Id: "t1"}}, nil
}
func (c *crmStub) ReopenTask(_ context.Context, in *crmv1.ReopenTaskRequest, _ ...grpc.CallOption) (*crmv1.ReopenTaskResponse, error) {
	if c.onReopen != nil {
		return c.onReopen(in)
	}
	return &crmv1.ReopenTaskResponse{Task: &crmv1.Task{Id: "t1"}}, nil
}
func (c *crmStub) CancelTask(_ context.Context, in *crmv1.CancelTaskRequest, _ ...grpc.CallOption) (*crmv1.CancelTaskResponse, error) {
	if c.onCancel != nil {
		return c.onCancel(in)
	}
	return &crmv1.CancelTaskResponse{}, nil
}
func (c *crmStub) ListGuests(_ context.Context, in *crmv1.ListGuestsRequest, _ ...grpc.CallOption) (*crmv1.ListGuestsResponse, error) {
	if c.onListGuest != nil {
		return c.onListGuest(in)
	}
	return &crmv1.ListGuestsResponse{}, nil
}
func (c *crmStub) GetGuest(_ context.Context, in *crmv1.GetGuestRequest, _ ...grpc.CallOption) (*crmv1.GetGuestResponse, error) {
	if c.onGetGuest != nil {
		return c.onGetGuest(in)
	}
	return &crmv1.GetGuestResponse{}, nil
}

// crmUserMock supports the two user lookups venue_crm.go performs.
type crmUserMock struct {
	userv1.UserServiceClient
	byID     map[string]*userv1.UserResponse
	byEmail  map[string]*userv1.UserResponse
	emailErr error
}

func (m *crmUserMock) GetUser(_ context.Context, in *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	if u, ok := m.byID[in.GetId()]; ok {
		return u, nil
	}
	return nil, status.Error(codes.NotFound, "no user")
}

func (m *crmUserMock) GetUserByEmail(_ context.Context, in *userv1.GetUserByEmailRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	if m.emailErr != nil {
		return nil, m.emailErr
	}
	if u, ok := m.byEmail[in.GetEmail()]; ok {
		return u, nil
	}
	return nil, status.Error(codes.NotFound, "no user")
}

func strptr(s string) *string { return &s }

type staffNotifierSpy struct {
	calls  int
	userID string
}

func (s *staffNotifierSpy) NotifyStaffInvited(_ context.Context, userID, _, _ string) {
	s.calls++
	s.userID = userID
}

type taskNotifierSpy struct {
	calls    int
	assignee string
}

func (s *taskNotifierSpy) NotifyTaskAssigned(_ context.Context, assigneeID, _, _, _ string) {
	s.calls++
	s.assignee = assigneeID
}

func newVenueHandler(crm crmv1.CRMServiceClient, user userv1.UserServiceClient, opts ...VenueHandlerOption) *VenueHandler {
	return NewVenueHandler(&reviewVenueMock{}, user, crm, noopUploader{}, opts...)
}

// --- ListVenueStaff ---

func TestListVenueStaff_Unauthorized(t *testing.T) {
	h := newVenueHandler(&crmStub{}, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.ListVenueStaff(rr, reviewReq(http.MethodGet, "/staff", "", map[string]string{"venueId": "v1"}, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestListVenueStaff_EnrichesProfiles(t *testing.T) {
	crm := &crmStub{onListStaff: func(in *crmv1.ListStaffRequest) (*crmv1.ListStaffResponse, error) {
		return &crmv1.ListStaffResponse{Members: []*crmv1.StaffMember{
			{UserId: "u2", Role: "manager", InvitedBy: "actor", CreatedAt: timestamppb.Now()},
		}}, nil
	}}
	user := &crmUserMock{byID: map[string]*userv1.UserResponse{
		"u2":    {Id: "u2", Name: "Bob", Email: "bob@x.io"},
		"actor": {Id: "actor", Name: "Boss", Email: "boss@x.io"},
	}}
	h := newVenueHandler(crm, user)
	rr := httptest.NewRecorder()
	h.ListVenueStaff(rr, reviewReq(http.MethodGet, "/staff", "", map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Staff []map[string]any `json:"staff"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	row := body.Staff[0]
	if row["user_name"] != "Bob" || row["inviter_name"] != "Boss" {
		t.Fatalf("enrichment missing: %v", row)
	}
	if row["inviter_is_you"] != true {
		t.Fatalf("inviter_is_you not set: %v", row)
	}
}

func TestListVenueStaff_GRPCError(t *testing.T) {
	crm := &crmStub{onListStaff: func(*crmv1.ListStaffRequest) (*crmv1.ListStaffResponse, error) {
		return nil, status.Error(codes.PermissionDenied, "no")
	}}
	h := newVenueHandler(crm, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.ListVenueStaff(rr, reviewReq(http.MethodGet, "/staff", "", map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- AddVenueStaffByEmail ---

func TestAddVenueStaff_NilUserClient(t *testing.T) {
	h := newVenueHandler(&crmStub{}, nil)
	rr := httptest.NewRecorder()
	h.AddVenueStaffByEmail(rr, reviewReq(http.MethodPost, "/staff", `{"email":"a@b.c","role":"manager"}`, map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code == http.StatusOK || rr.Code == http.StatusCreated {
		t.Fatalf("expected failure, got %d", rr.Code)
	}
}

func TestAddVenueStaff_EmptyEmail(t *testing.T) {
	h := newVenueHandler(&crmStub{}, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.AddVenueStaffByEmail(rr, reviewReq(http.MethodPost, "/staff", `{"email":"  ","role":"manager"}`, map[string]string{"venueId": "v1"}, "actor"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestEmailRequired.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestAddVenueStaff_EmailNotRegistered(t *testing.T) {
	h := newVenueHandler(&crmStub{}, &crmUserMock{byEmail: map[string]*userv1.UserResponse{}})
	rr := httptest.NewRecorder()
	h.AddVenueStaffByEmail(rr, reviewReq(http.MethodPost, "/staff", `{"email":"missing@x.io","role":"manager"}`, map[string]string{"venueId": "v1"}, "actor"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayCrmStaffEmailNotRegistered.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestAddVenueStaff_Success_FiresNotifier(t *testing.T) {
	spy := &staffNotifierSpy{}
	user := &crmUserMock{byEmail: map[string]*userv1.UserResponse{
		"new@x.io": {Id: "u9", Email: "new@x.io"},
	}}
	var added *crmv1.AddStaffRequest
	crm := &crmStub{onAddStaff: func(in *crmv1.AddStaffRequest) (*crmv1.AddStaffResponse, error) {
		added = in
		return &crmv1.AddStaffResponse{}, nil
	}}
	h := newVenueHandler(crm, user, WithStaffInviteNotifier(spy))
	rr := httptest.NewRecorder()
	h.AddVenueStaffByEmail(rr, reviewReq(http.MethodPost, "/staff", `{"email":"NEW@x.io","role":"manager"}`, map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if added.GetUserId() != "u9" || added.GetRole() != "manager" {
		t.Fatalf("add req = %+v", added)
	}
	if spy.calls != 1 || spy.userID != "u9" {
		t.Fatalf("notifier: calls=%d user=%s", spy.calls, spy.userID)
	}
}

func TestAddVenueStaff_LookupError(t *testing.T) {
	user := &crmUserMock{emailErr: status.Error(codes.Internal, "boom")}
	h := newVenueHandler(&crmStub{}, user)
	rr := httptest.NewRecorder()
	h.AddVenueStaffByEmail(rr, reviewReq(http.MethodPost, "/staff", `{"email":"a@x.io"}`, map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- RemoveVenueStaff ---

func TestRemoveVenueStaff_Success(t *testing.T) {
	var got *crmv1.RemoveStaffRequest
	crm := &crmStub{onRemove: func(in *crmv1.RemoveStaffRequest) (*crmv1.RemoveStaffResponse, error) {
		got = in
		return &crmv1.RemoveStaffResponse{}, nil
	}}
	h := newVenueHandler(crm, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.RemoveVenueStaff(rr, reviewReq(http.MethodDelete, "/staff/u2", "", map[string]string{"venueId": "v1", "userId": "u2"}, "actor"))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if got.GetUserId() != "u2" || got.GetActorId() != "actor" {
		t.Fatalf("remove req = %+v", got)
	}
}

func TestRemoveVenueStaff_Unauthorized(t *testing.T) {
	h := newVenueHandler(&crmStub{}, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.RemoveVenueStaff(rr, reviewReq(http.MethodDelete, "/staff/u2", "", map[string]string{"venueId": "v1", "userId": "u2"}, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- CRM tasks ---

func TestListVenueCRMTasks_Success(t *testing.T) {
	crm := &crmStub{onListTasks: func(in *crmv1.ListTasksRequest) (*crmv1.ListTasksResponse, error) {
		if in.GetStatus() != "open" {
			t.Fatalf("status filter = %s", in.GetStatus())
		}
		return &crmv1.ListTasksResponse{Tasks: []*crmv1.Task{{Id: "t1", Title: "A"}}}, nil
	}}
	h := newVenueHandler(crm, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.ListVenueCRMTasks(rr, reviewReq(http.MethodGet, "/tasks?status=open", "", map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Tasks []map[string]any `json:"tasks"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Tasks) != 1 || body.Tasks[0]["title"] != "A" {
		t.Fatalf("tasks = %v", body.Tasks)
	}
}

func TestCreateVenueCRMTask_InvalidDueAt(t *testing.T) {
	h := newVenueHandler(&crmStub{}, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.CreateVenueCRMTask(rr, reviewReq(http.MethodPost, "/tasks", `{"title":"x","due_at":"not-a-date"}`, map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestCreateVenueCRMTask_Success_FiresNotifier(t *testing.T) {
	spy := &taskNotifierSpy{}
	crm := &crmStub{onCreate: func(in *crmv1.CreateTaskRequest) (*crmv1.CreateTaskResponse, error) {
		if in.GetBookingId() != "b1" || in.GetAssigneeUserId() != "assignee" {
			t.Fatalf("optional fields not forwarded: %+v", in)
		}
		return &crmv1.CreateTaskResponse{Task: &crmv1.Task{Id: "t1", Title: "Clean", AssigneeUserId: strptr("assignee")}}, nil
	}}
	h := newVenueHandler(crm, &crmUserMock{}, WithCRMTaskNotifier(spy))
	rr := httptest.NewRecorder()
	body := `{"title":"Clean","booking_id":"b1","assignee_user_id":"assignee","due_at":"2026-01-02T15:04:05Z"}`
	h.CreateVenueCRMTask(rr, reviewReq(http.MethodPost, "/tasks", body, map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if spy.calls != 1 || spy.assignee != "assignee" {
		t.Fatalf("notifier: calls=%d assignee=%s", spy.calls, spy.assignee)
	}
}

func TestCreateVenueCRMTask_NilTask(t *testing.T) {
	crm := &crmStub{onCreate: func(*crmv1.CreateTaskRequest) (*crmv1.CreateTaskResponse, error) {
		return &crmv1.CreateTaskResponse{}, nil
	}}
	h := newVenueHandler(crm, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.CreateVenueCRMTask(rr, reviewReq(http.MethodPost, "/tasks", `{"title":"x"}`, map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestUpdateVenueCRMTask_Success(t *testing.T) {
	crm := &crmStub{onUpdate: func(in *crmv1.UpdateTaskRequest) (*crmv1.UpdateTaskResponse, error) {
		if in.GetTaskId() != "t7" {
			t.Fatalf("task id = %s", in.GetTaskId())
		}
		return &crmv1.UpdateTaskResponse{Task: &crmv1.Task{Id: "t7", Title: "Updated"}}, nil
	}}
	h := newVenueHandler(crm, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.UpdateVenueCRMTask(rr, reviewReq(http.MethodPut, "/tasks/t7", `{"title":"Updated","assignee_user_id":"a"}`, map[string]string{"venueId": "v1", "taskId": "t7"}, "actor"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestUpdateVenueCRMTask_InvalidDueAt(t *testing.T) {
	h := newVenueHandler(&crmStub{}, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.UpdateVenueCRMTask(rr, reviewReq(http.MethodPut, "/tasks/t7", `{"due_at":"bad"}`, map[string]string{"venueId": "v1", "taskId": "t7"}, "actor"))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestCompleteReopenCancelCRMTask(t *testing.T) {
	h := newVenueHandler(&crmStub{}, &crmUserMock{})

	t.Run("complete", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.CompleteVenueCRMTask(rr, reviewReq(http.MethodPost, "/tasks/t1/complete", "", map[string]string{"venueId": "v1", "taskId": "t1"}, "actor"))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rr.Code)
		}
	})
	t.Run("reopen", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.ReopenVenueCRMTask(rr, reviewReq(http.MethodPost, "/tasks/t1/reopen", "", map[string]string{"venueId": "v1", "taskId": "t1"}, "actor"))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d", rr.Code)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.CancelVenueCRMTask(rr, reviewReq(http.MethodDelete, "/tasks/t1", "", map[string]string{"venueId": "v1", "taskId": "t1"}, "actor"))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rr.Code)
		}
	})
	t.Run("complete_unauthorized", func(t *testing.T) {
		rr := httptest.NewRecorder()
		h.CompleteVenueCRMTask(rr, reviewReq(http.MethodPost, "/tasks/t1/complete", "", map[string]string{"venueId": "v1", "taskId": "t1"}, ""))
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rr.Code)
		}
	})
}

// --- guests ---

func TestListVenueGuests_Success(t *testing.T) {
	crm := &crmStub{onListGuest: func(in *crmv1.ListGuestsRequest) (*crmv1.ListGuestsResponse, error) {
		return &crmv1.ListGuestsResponse{
			Total:  1,
			Guests: []*crmv1.GuestProfile{{UserId: "g1", VenueId: "v1", BookingsCount: 3}},
		}, nil
	}}
	user := &crmUserMock{byID: map[string]*userv1.UserResponse{"g1": {Id: "g1", Name: "Guest", Email: "g@x.io"}}}
	h := newVenueHandler(crm, user)
	rr := httptest.NewRecorder()
	h.ListVenueGuests(rr, reviewReq(http.MethodGet, "/guests?limit=10&offset=0&segment=vip", "", map[string]string{"venueId": "v1"}, "actor"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Guests []map[string]any `json:"guests"`
		Total  int              `json:"total"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Total != 1 || body.Guests[0]["user_name"] != "Guest" {
		t.Fatalf("guests = %+v", body)
	}
}

func TestGetVenueGuest_Success(t *testing.T) {
	crm := &crmStub{onGetGuest: func(in *crmv1.GetGuestRequest) (*crmv1.GetGuestResponse, error) {
		return &crmv1.GetGuestResponse{
			Profile: &crmv1.GuestProfile{UserId: "g1", VenueId: "v1"},
			RecentBookings: []*crmv1.GuestBookingSummary{
				{BookingId: "b1", Status: "completed", TotalPrice: 500, Guests: 2, VisitDate: timestamppb.New(time.Now())},
			},
		}, nil
	}}
	user := &crmUserMock{byID: map[string]*userv1.UserResponse{"g1": {Id: "g1", Name: "Guest"}}}
	h := newVenueHandler(crm, user)
	rr := httptest.NewRecorder()
	h.GetVenueGuest(rr, reviewReq(http.MethodGet, "/guests/g1", "", map[string]string{"venueId": "v1", "userId": "g1"}, "actor"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Guest          map[string]any   `json:"guest"`
		RecentBookings []map[string]any `json:"recent_bookings"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if len(body.RecentBookings) != 1 || body.RecentBookings[0]["booking_id"] != "b1" {
		t.Fatalf("recent bookings = %v", body.RecentBookings)
	}
}

func TestGetVenueGuest_Error(t *testing.T) {
	crm := &crmStub{onGetGuest: func(*crmv1.GetGuestRequest) (*crmv1.GetGuestResponse, error) {
		return nil, status.Error(codes.NotFound, "no guest")
	}}
	h := newVenueHandler(crm, &crmUserMock{})
	rr := httptest.NewRecorder()
	h.GetVenueGuest(rr, reviewReq(http.MethodGet, "/guests/g1", "", map[string]string{"venueId": "v1", "userId": "g1"}, "actor"))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- pure helpers ---

func TestParseCRMDueAt(t *testing.T) {
	if ts, ok := parseCRMDueAt(nil); !ok || ts != nil {
		t.Fatalf("nil: got (%v,%v)", ts, ok)
	}
	blank := "  "
	if ts, ok := parseCRMDueAt(&blank); !ok || ts != nil {
		t.Fatalf("blank: got (%v,%v)", ts, ok)
	}
	good := "2026-01-02T15:04:05Z"
	if ts, ok := parseCRMDueAt(&good); !ok || ts == nil {
		t.Fatalf("valid: got (%v,%v)", ts, ok)
	}
	bad := "nope"
	if ts, ok := parseCRMDueAt(&bad); ok || ts != nil {
		t.Fatalf("invalid: got (%v,%v)", ts, ok)
	}
}

func TestCRMTaskToMap_FullAndMinimal(t *testing.T) {
	now := timestamppb.Now()
	full := crmTaskToMap(&crmv1.Task{
		Id: "t1", VenueId: "v1", Title: "T", Body: "B", Status: "open", Priority: "high",
		CreatedBy: "u1", CreatedAt: now, UpdatedAt: now,
		BookingId: strptr("b1"), AssigneeUserId: strptr("a1"), DueAt: now, CompletedBy: "c1", CompletedAt: now,
	})
	for _, k := range []string{"booking_id", "assignee_user_id", "due_at", "completed_by", "completed_at"} {
		if _, ok := full[k]; !ok {
			t.Fatalf("missing optional key %q", k)
		}
	}
	minimal := crmTaskToMap(&crmv1.Task{Id: "t2", CreatedAt: now, UpdatedAt: now})
	for _, k := range []string{"booking_id", "assignee_user_id", "due_at", "completed_by", "completed_at"} {
		if _, ok := minimal[k]; ok {
			t.Fatalf("unexpected optional key %q on minimal task", k)
		}
	}
}

func TestCRMGuestToMap_NilSegments(t *testing.T) {
	m := crmGuestToMap(&crmv1.GuestProfile{UserId: "g1"}, venueStaffUserDisplay{name: "N", email: "e@x.io"})
	if seg, ok := m["segments"].([]string); !ok || seg == nil {
		t.Fatalf("segments should be non-nil slice: %v", m["segments"])
	}
	if m["user_name"] != "N" || m["user_email"] != "e@x.io" {
		t.Fatalf("display not applied: %v", m)
	}
}
