package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	crmv1 "github.com/tienlao/agregator/gen/go/crm/v1"
	masterv1 "github.com/tienlao/agregator/gen/go/master/v1"
	reviewv1 "github.com/tienlao/agregator/gen/go/review/v1"
	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	venuev1 "github.com/tienlao/agregator/gen/go/venue/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- mocks (interface embedding: only the exercised methods are overridden;
// any other call panics on the nil embedded interface, which tests never hit) ---

type reviewSvcMock struct {
	reviewv1.ReviewServiceClient
	onCreate   func(*reviewv1.CreateReviewRequest) (*reviewv1.ReviewResponse, error)
	onListVen  func(*reviewv1.ListVenueReviewsRequest) (*reviewv1.ListReviewsResponse, error)
	onListMas  func(*reviewv1.ListMasterReviewsRequest) (*reviewv1.ListReviewsResponse, error)
	onMasRate  func(*reviewv1.GetMasterRatingRequest) (*reviewv1.MasterRatingResponse, error)
	onSummary  func(*reviewv1.GetVenueReviewSummaryRequest) (*reviewv1.VenueReviewSummaryResponse, error)
	onReply    func(*reviewv1.ReplyToReviewRequest) (*reviewv1.ReviewReply, error)
	onDelReply func(*reviewv1.DeleteReviewReplyRequest) (*reviewv1.DeleteReviewReplyResponse, error)
}

func (m *reviewSvcMock) CreateReview(_ context.Context, in *reviewv1.CreateReviewRequest, _ ...grpc.CallOption) (*reviewv1.ReviewResponse, error) {
	if m.onCreate != nil {
		return m.onCreate(in)
	}
	return &reviewv1.ReviewResponse{Id: "rev-1", UserId: in.GetUserId(), VenueId: in.GetVenueId(), MasterId: in.GetMasterId(), Rating: in.GetRating(), Text: in.GetText(), IsAnonymous: in.GetIsAnonymous(), UserName: in.GetUserName()}, nil
}

func (m *reviewSvcMock) ListVenueReviews(_ context.Context, in *reviewv1.ListVenueReviewsRequest, _ ...grpc.CallOption) (*reviewv1.ListReviewsResponse, error) {
	if m.onListVen != nil {
		return m.onListVen(in)
	}
	return &reviewv1.ListReviewsResponse{}, nil
}

func (m *reviewSvcMock) ListMasterReviews(_ context.Context, in *reviewv1.ListMasterReviewsRequest, _ ...grpc.CallOption) (*reviewv1.ListReviewsResponse, error) {
	if m.onListMas != nil {
		return m.onListMas(in)
	}
	return &reviewv1.ListReviewsResponse{}, nil
}

func (m *reviewSvcMock) GetMasterRating(_ context.Context, in *reviewv1.GetMasterRatingRequest, _ ...grpc.CallOption) (*reviewv1.MasterRatingResponse, error) {
	if m.onMasRate != nil {
		return m.onMasRate(in)
	}
	return &reviewv1.MasterRatingResponse{}, nil
}

func (m *reviewSvcMock) GetVenueReviewSummary(_ context.Context, in *reviewv1.GetVenueReviewSummaryRequest, _ ...grpc.CallOption) (*reviewv1.VenueReviewSummaryResponse, error) {
	if m.onSummary != nil {
		return m.onSummary(in)
	}
	return &reviewv1.VenueReviewSummaryResponse{}, nil
}

func (m *reviewSvcMock) ReplyToReview(_ context.Context, in *reviewv1.ReplyToReviewRequest, _ ...grpc.CallOption) (*reviewv1.ReviewReply, error) {
	if m.onReply != nil {
		return m.onReply(in)
	}
	return &reviewv1.ReviewReply{Body: in.GetBody()}, nil
}

func (m *reviewSvcMock) DeleteReviewReply(_ context.Context, in *reviewv1.DeleteReviewReplyRequest, _ ...grpc.CallOption) (*reviewv1.DeleteReviewReplyResponse, error) {
	if m.onDelReply != nil {
		return m.onDelReply(in)
	}
	return &reviewv1.DeleteReviewReplyResponse{}, nil
}

type reviewUserMock struct {
	userv1.UserServiceClient
	onGet func(id string) (*userv1.UserResponse, error)
}

func (m *reviewUserMock) GetUser(_ context.Context, in *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
	if m.onGet != nil {
		return m.onGet(in.GetId())
	}
	return &userv1.UserResponse{Id: in.GetId(), Name: "Ivan"}, nil
}

type reviewCRMMock struct {
	crmv1.CRMServiceClient
	access string
	err    error
}

func (m *reviewCRMMock) GetManagementAccess(_ context.Context, in *crmv1.GetManagementAccessRequest, _ ...grpc.CallOption) (*crmv1.GetManagementAccessResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &crmv1.GetManagementAccessResponse{Access: m.access}, nil
}

type reviewVenueMock struct {
	venuev1.VenueServiceClient
	onGet func(id string) (*venuev1.VenueResponse, error)
}

func (m *reviewVenueMock) GetVenue(_ context.Context, in *venuev1.GetVenueRequest, _ ...grpc.CallOption) (*venuev1.VenueResponse, error) {
	if m.onGet != nil {
		return m.onGet(in.GetId())
	}
	return &venuev1.VenueResponse{Id: in.GetId()}, nil
}

type reviewMasterMock struct {
	masterv1.MasterServiceClient
	onGet func(id string) (*masterv1.MasterResponse, error)
}

func (m *reviewMasterMock) GetMaster(_ context.Context, in *masterv1.GetMasterRequest, _ ...grpc.CallOption) (*masterv1.MasterResponse, error) {
	if m.onGet != nil {
		return m.onGet(in.GetId())
	}
	return &masterv1.MasterResponse{}, nil
}

type capturingOwnerNotifier struct {
	calls int
	owner string
	venue string
}

func (n *capturingOwnerNotifier) NotifyVenueReviewCreated(_ context.Context, ownerID, venueID, _ string, _ int32) {
	n.calls++
	n.owner = ownerID
	n.venue = venueID
}

type capturingMasterNotifier struct {
	calls     int
	masterUID string
}

func (n *capturingMasterNotifier) NotifyMasterReviewCreated(_ context.Context, masterUserID, _, _ string, _ int32) {
	n.calls++
	n.masterUID = masterUserID
}

// --- test helpers ---

func reviewReq(method, target, body string, params map[string]string, userID string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	if userID != "" {
		r = r.WithContext(middleware.WithUserID(r.Context(), userID))
	}
	return r
}

func decodeCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if c, ok := body["code"].(string); ok {
		return c
	}
	return ""
}

// --- Create ---

func TestReviewCreate_Unauthorized(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/reviews", `{"venue_id":"v1","rating":5}`, nil, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := decodeCode(t, rr); got != apicatalog.GatewayAuthUnauthorized.Code {
		t.Fatalf("code = %s", got)
	}
}

func TestReviewCreate_InvalidBody(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/reviews", `{not-json`, nil, "u1"))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestReviewCreate_Success_ResolvesUserName(t *testing.T) {
	var gotReq *reviewv1.CreateReviewRequest
	svc := &reviewSvcMock{onCreate: func(in *reviewv1.CreateReviewRequest) (*reviewv1.ReviewResponse, error) {
		gotReq = in
		return &reviewv1.ReviewResponse{Id: "rev-9", UserId: in.GetUserId(), VenueId: in.GetVenueId(), Rating: in.GetRating()}, nil
	}}
	user := &reviewUserMock{onGet: func(id string) (*userv1.UserResponse, error) {
		return &userv1.UserResponse{Id: id, Name: "  Petya  "}, nil
	}}
	h := NewReviewHandler(svc, user, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/reviews", `{"venue_id":"v1","rating":4,"text":"ok"}`, nil, "u1"))

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if gotReq.GetUserName() != "Petya" {
		t.Fatalf("user name not trimmed/resolved: %q", gotReq.GetUserName())
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["id"] != "rev-9" {
		t.Fatalf("id = %v", body["id"])
	}
}

func TestReviewCreate_BookingNotVerified(t *testing.T) {
	svc := &reviewSvcMock{onCreate: func(*reviewv1.CreateReviewRequest) (*reviewv1.ReviewResponse, error) {
		return nil, status.Error(codes.FailedPrecondition, "booking is not confirmed by platform")
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/reviews", `{"venue_id":"v1","rating":4}`, nil, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayReviewBookingNotVerified.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestReviewCreate_GRPCErrorPassthrough(t *testing.T) {
	svc := &reviewSvcMock{onCreate: func(*reviewv1.CreateReviewRequest) (*reviewv1.ReviewResponse, error) {
		return nil, status.Error(codes.Internal, "boom")
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/reviews", `{"venue_id":"v1","rating":4}`, nil, "u1"))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestReviewCreate_FiresOwnerNotification(t *testing.T) {
	notifier := &capturingOwnerNotifier{}
	venue := &reviewVenueMock{onGet: func(id string) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: id, OwnerId: "owner-7", Name: "Баня"}, nil
	}}
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{}, WithReviewOwnerNotifier(venue, notifier))
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/reviews", `{"venue_id":"v1","rating":5}`, nil, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if notifier.calls != 1 || notifier.owner != "owner-7" || notifier.venue != "v1" {
		t.Fatalf("owner notifier: calls=%d owner=%s venue=%s", notifier.calls, notifier.owner, notifier.venue)
	}
}

func TestReviewCreate_SkipsOwnerNotificationForSelfReview(t *testing.T) {
	notifier := &capturingOwnerNotifier{}
	venue := &reviewVenueMock{onGet: func(id string) (*venuev1.VenueResponse, error) {
		return &venuev1.VenueResponse{Id: id, OwnerId: "u1"}, nil // author == owner
	}}
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{}, WithReviewOwnerNotifier(venue, notifier))
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/reviews", `{"venue_id":"v1","rating":5}`, nil, "u1"))
	if notifier.calls != 0 {
		t.Fatalf("expected no notification for self-review, got %d", notifier.calls)
	}
}

func TestReviewCreate_FiresMasterNotification(t *testing.T) {
	notifier := &capturingMasterNotifier{}
	master := &reviewMasterMock{onGet: func(id string) (*masterv1.MasterResponse, error) {
		return &masterv1.MasterResponse{Master: &masterv1.Master{Id: id, UserId: "master-uid", DisplayName: "Мастер"}}, nil
	}}
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{}, WithReviewMasterNotifier(master, notifier))
	rr := httptest.NewRecorder()
	h.Create(rr, reviewReq(http.MethodPost, "/reviews", `{"master_id":"m1","rating":5}`, nil, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if notifier.calls != 1 || notifier.masterUID != "master-uid" {
		t.Fatalf("master notifier: calls=%d uid=%s", notifier.calls, notifier.masterUID)
	}
}

// --- CreateForVenue ---

func TestReviewCreateForVenue_MissingVenueID(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.CreateForVenue(rr, reviewReq(http.MethodPost, "/reviews", `{"rating":5}`, nil, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayReviewVenueIdRequired.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestReviewCreateForVenue_Success(t *testing.T) {
	var gotReq *reviewv1.CreateReviewRequest
	svc := &reviewSvcMock{onCreate: func(in *reviewv1.CreateReviewRequest) (*reviewv1.ReviewResponse, error) {
		gotReq = in
		return &reviewv1.ReviewResponse{Id: "r1"}, nil
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.CreateForVenue(rr, reviewReq(http.MethodPost, "/reviews", `{"rating":5}`, map[string]string{"venueId": "v-42"}, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if gotReq.GetVenueId() != "v-42" {
		t.Fatalf("venue id = %s", gotReq.GetVenueId())
	}
}

func TestReviewCreateForVenue_Unauthorized(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.CreateForVenue(rr, reviewReq(http.MethodPost, "/reviews", `{"rating":5}`, map[string]string{"venueId": "v1"}, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- CreateForMaster ---

func TestReviewCreateForMaster_MissingMasterID(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.CreateForMaster(rr, reviewReq(http.MethodPost, "/reviews", `{"rating":5}`, nil, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestInvalidBody.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestReviewCreateForMaster_Success(t *testing.T) {
	var gotReq *reviewv1.CreateReviewRequest
	svc := &reviewSvcMock{onCreate: func(in *reviewv1.CreateReviewRequest) (*reviewv1.ReviewResponse, error) {
		gotReq = in
		return &reviewv1.ReviewResponse{Id: "r1"}, nil
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.CreateForMaster(rr, reviewReq(http.MethodPost, "/reviews", `{"rating":3}`, map[string]string{"masterId": "m-9"}, "u1"))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	if gotReq.GetMasterId() != "m-9" {
		t.Fatalf("master id = %s", gotReq.GetMasterId())
	}
}

// --- ListByVenue / ListByMaster ---

func TestReviewListByVenue_ResolvesMissingNames(t *testing.T) {
	svc := &reviewSvcMock{onListVen: func(*reviewv1.ListVenueReviewsRequest) (*reviewv1.ListReviewsResponse, error) {
		return &reviewv1.ListReviewsResponse{
			Total: 2,
			Reviews: []*reviewv1.ReviewResponse{
				{Id: "a", UserId: "u1", UserName: ""},      // needs resolve
				{Id: "b", UserId: "u2", IsAnonymous: true}, // anon: no resolve
			},
		}, nil
	}}
	calls := 0
	user := &reviewUserMock{onGet: func(id string) (*userv1.UserResponse, error) {
		calls++
		return &userv1.UserResponse{Id: id, Name: "Resolved"}, nil
	}}
	h := NewReviewHandler(svc, user, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.ListByVenue(rr, reviewReq(http.MethodGet, "/reviews?page=1&page_size=10", "", map[string]string{"venueId": "v1"}, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if calls != 1 {
		t.Fatalf("expected 1 user lookup (anon skipped), got %d", calls)
	}
	var body struct {
		Reviews []map[string]any `json:"reviews"`
		Total   int              `json:"total"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Total != 2 || len(body.Reviews) != 2 {
		t.Fatalf("total=%d len=%d", body.Total, len(body.Reviews))
	}
	if body.Reviews[0]["user_name"] != "Resolved" {
		t.Fatalf("name0 = %v", body.Reviews[0]["user_name"])
	}
}

func TestReviewListByVenue_InvalidPage(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.ListByVenue(rr, reviewReq(http.MethodGet, "/reviews?page=abc", "", map[string]string{"venueId": "v1"}, ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestReviewListByMaster_Success(t *testing.T) {
	svc := &reviewSvcMock{onListMas: func(in *reviewv1.ListMasterReviewsRequest) (*reviewv1.ListReviewsResponse, error) {
		if in.GetMasterId() != "m1" {
			t.Fatalf("master id = %s", in.GetMasterId())
		}
		return &reviewv1.ListReviewsResponse{Total: 1, Reviews: []*reviewv1.ReviewResponse{{Id: "x", UserName: "Given"}}}, nil
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.ListByMaster(rr, reviewReq(http.MethodGet, "/reviews", "", map[string]string{"masterId": "m1"}, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- MasterRating ---

func TestReviewMasterRating_Success(t *testing.T) {
	svc := &reviewSvcMock{onMasRate: func(*reviewv1.GetMasterRatingRequest) (*reviewv1.MasterRatingResponse, error) {
		return &reviewv1.MasterRatingResponse{MasterId: "m1", AvgRating: 4.5, ReviewCount: 12}, nil
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.MasterRating(rr, reviewReq(http.MethodGet, "/rating", "", map[string]string{"masterId": "m1"}, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["review_count"].(float64) != 12 {
		t.Fatalf("review_count = %v", body["review_count"])
	}
}

func TestReviewMasterRating_Error(t *testing.T) {
	svc := &reviewSvcMock{onMasRate: func(*reviewv1.GetMasterRatingRequest) (*reviewv1.MasterRatingResponse, error) {
		return nil, status.Error(codes.NotFound, "nope")
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.MasterRating(rr, reviewReq(http.MethodGet, "/rating", "", map[string]string{"masterId": "m1"}, ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- owner cabinet: ensureVenueManageAccess via ListForOwner ---

func TestReviewListForOwner_Unauthorized(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{})
	rr := httptest.NewRecorder()
	h.ListForOwner(rr, reviewReq(http.MethodGet, "/owner", "", map[string]string{"venueId": "v1"}, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestReviewListForOwner_Forbidden_NoAccess(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{access: ""})
	rr := httptest.NewRecorder()
	h.ListForOwner(rr, reviewReq(http.MethodGet, "/owner", "", map[string]string{"venueId": "v1"}, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayReviewForbidden.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestReviewListForOwner_Forbidden_NilCRM(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, nil)
	rr := httptest.NewRecorder()
	h.ListForOwner(rr, reviewReq(http.MethodGet, "/owner", "", map[string]string{"venueId": "v1"}, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayReviewForbidden.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestReviewListForOwner_Success(t *testing.T) {
	svc := &reviewSvcMock{onListVen: func(in *reviewv1.ListVenueReviewsRequest) (*reviewv1.ListReviewsResponse, error) {
		if !in.GetOnlyUnanswered() {
			t.Fatalf("expected only_unanswered=true")
		}
		return &reviewv1.ListReviewsResponse{Total: 1, Reviews: []*reviewv1.ReviewResponse{{Id: "x", UserName: "N"}}}, nil
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{access: "owner"})
	rr := httptest.NewRecorder()
	h.ListForOwner(rr, reviewReq(http.MethodGet, "/owner?only_unanswered=true", "", map[string]string{"venueId": "v1"}, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestReviewListForOwner_CRMError(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{err: status.Error(codes.Unavailable, "down")})
	rr := httptest.NewRecorder()
	h.ListForOwner(rr, reviewReq(http.MethodGet, "/owner", "", map[string]string{"venueId": "v1"}, "u1"))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rr.Code)
	}
}

// --- SummaryForOwner ---

func TestReviewSummaryForOwner_Success(t *testing.T) {
	svc := &reviewSvcMock{onSummary: func(*reviewv1.GetVenueReviewSummaryRequest) (*reviewv1.VenueReviewSummaryResponse, error) {
		return &reviewv1.VenueReviewSummaryResponse{AvgRating: 4.2, ReviewCount: 5, UnansweredCount: 2}, nil
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{access: "manager"})
	rr := httptest.NewRecorder()
	h.SummaryForOwner(rr, reviewReq(http.MethodGet, "/owner/summary", "", map[string]string{"venueId": "v1"}, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["unanswered_count"].(float64) != 2 {
		t.Fatalf("unanswered = %v", body["unanswered_count"])
	}
}

// --- Reply / DeleteReply ---

func TestReviewReply_InvalidReviewID(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{access: "owner"})
	rr := httptest.NewRecorder()
	h.Reply(rr, reviewReq(http.MethodPut, "/reply", `{"text":"hi"}`, map[string]string{"venueId": "v1", "reviewId": "not-a-uuid"}, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestInvalidReviewId.Code {
		t.Fatalf("code = %s (status %d)", got, rr.Code)
	}
}

func TestReviewReply_Success(t *testing.T) {
	const rid = "11111111-1111-1111-1111-111111111111"
	var gotReq *reviewv1.ReplyToReviewRequest
	svc := &reviewSvcMock{onReply: func(in *reviewv1.ReplyToReviewRequest) (*reviewv1.ReviewReply, error) {
		gotReq = in
		return &reviewv1.ReviewReply{Body: in.GetBody()}, nil
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{access: "owner"})
	rr := httptest.NewRecorder()
	h.Reply(rr, reviewReq(http.MethodPut, "/reply", `{"text":"Спасибо"}`, map[string]string{"venueId": "v1", "reviewId": rid}, "u1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if gotReq.GetReviewId() != rid || gotReq.GetAuthorUserId() != "u1" || gotReq.GetBody() != "Спасибо" {
		t.Fatalf("reply req = %+v", gotReq)
	}
}

func TestReviewDeleteReply_Success(t *testing.T) {
	const rid = "22222222-2222-2222-2222-222222222222"
	var called bool
	svc := &reviewSvcMock{onDelReply: func(in *reviewv1.DeleteReviewReplyRequest) (*reviewv1.DeleteReviewReplyResponse, error) {
		called = true
		if in.GetReviewId() != rid {
			t.Fatalf("review id = %s", in.GetReviewId())
		}
		return &reviewv1.DeleteReviewReplyResponse{}, nil
	}}
	h := NewReviewHandler(svc, &reviewUserMock{}, &reviewCRMMock{access: "owner"})
	rr := httptest.NewRecorder()
	h.DeleteReply(rr, reviewReq(http.MethodDelete, "/reply", "", map[string]string{"venueId": "v1", "reviewId": rid}, "u1"))
	if rr.Code != http.StatusOK || !called {
		t.Fatalf("status = %d called=%v", rr.Code, called)
	}
}

func TestReviewDeleteReply_InvalidID(t *testing.T) {
	h := NewReviewHandler(&reviewSvcMock{}, &reviewUserMock{}, &reviewCRMMock{access: "owner"})
	rr := httptest.NewRecorder()
	h.DeleteReply(rr, reviewReq(http.MethodDelete, "/reply", "", map[string]string{"venueId": "v1", "reviewId": "bad"}, "u1"))
	if got := decodeCode(t, rr); got != apicatalog.GatewayRequestInvalidReviewId.Code {
		t.Fatalf("code = %s", got)
	}
}

// --- pure helpers: reviewToJSON / replyToJSON / handleReviewCreateError ---

func TestReviewToJSON_FallsBackToResponseName(t *testing.T) {
	m := reviewToJSON(&reviewv1.ReviewResponse{Id: "r", UserName: "Embedded", Rating: 5}, "")
	if m["user_name"] != "Embedded" {
		t.Fatalf("user_name = %v", m["user_name"])
	}
	if _, ok := m["owner_reply"]; ok {
		t.Fatalf("owner_reply should be absent when nil")
	}
}

func TestReviewToJSON_IncludesOwnerReply(t *testing.T) {
	m := reviewToJSON(&reviewv1.ReviewResponse{Id: "r", OwnerReply: &reviewv1.ReviewReply{Body: "thanks"}}, "Override")
	if m["user_name"] != "Override" {
		t.Fatalf("user_name = %v", m["user_name"])
	}
	reply, ok := m["owner_reply"].(map[string]any)
	if !ok || reply["body"] != "thanks" {
		t.Fatalf("owner_reply = %v", m["owner_reply"])
	}
}

func TestReplyToJSON_Nil(t *testing.T) {
	if replyToJSON(nil) != nil {
		t.Fatal("expected nil for nil reply")
	}
}

func TestHandleReviewCreateError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		want     bool
		wantHTTP int
	}{
		{"non-status error", context.Canceled, false, 0},
		{"wrong code", status.Error(codes.Internal, "booking is not confirmed by platform"), false, 0},
		{"right code wrong msg", status.Error(codes.FailedPrecondition, "other"), false, 0},
		{"match", status.Error(codes.FailedPrecondition, "Booking is not confirmed by platform"), true, http.StatusConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			got := handleReviewCreateError(rr, tc.err)
			if got != tc.want {
				t.Fatalf("handled = %v, want %v", got, tc.want)
			}
			if tc.want && rr.Code != tc.wantHTTP {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantHTTP)
			}
		})
	}
}
