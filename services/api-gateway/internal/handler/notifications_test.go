package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	notificationv1 "github.com/tienlao/agregator/gen/go/notification/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

// ── notificationToJSON ───────────────────────────────────────────────────────

func TestNotificationToJSON_NilReturnsNil(t *testing.T) {
	if got := notificationToJSON(nil); got != nil {
		t.Fatalf("notificationToJSON(nil) = %v, want nil", got)
	}
}

func TestNotificationToJSON_LowercasesIDsAndOmitsEmptyOptional(t *testing.T) {
	out := notificationToJSON(&notificationv1.Notification{
		Id:     "ABC-123",
		UserId: "USER-9",
		Type:   "booking",
		Title:  "Новая бронь",
		Body:   "Детали",
		Read:   false,
	})
	if out["id"] != "abc-123" {
		t.Fatalf("id should be lowercased: %v", out["id"])
	}
	if out["user_id"] != "user-9" {
		t.Fatalf("user_id should be lowercased: %v", out["user_id"])
	}
	for _, k := range []string{"data", "created_at", "read_at"} {
		if _, ok := out[k]; ok {
			t.Fatalf("optional %q should be omitted when empty: %v", k, out)
		}
	}
}

func TestNotificationToJSON_IncludesPopulatedOptional(t *testing.T) {
	out := notificationToJSON(&notificationv1.Notification{
		Id:        "x",
		Data:      `{"booking_id":"b1"}`,
		Read:      true,
		CreatedAt: timestamppb.Now(),
		ReadAt:    timestamppb.Now(),
	})
	for _, k := range []string{"data", "created_at", "read_at"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("populated %q should be present: %v", k, out)
		}
	}
	if out["read"] != true {
		t.Fatalf("read = %v, want true", out["read"])
	}
}

// ── REST handlers ────────────────────────────────────────────────────────────

// notifClientStub implements the narrow notificationGatewayClient interface.
type notifClientStub struct {
	listReq *notificationv1.ListRequest
	listOut *notificationv1.ListResponse
	listErr error

	unreadOut int32
	markOut   int32
	markErr   error
}

func (s *notifClientStub) Create(context.Context, *notificationv1.CreateRequest, ...grpc.CallOption) (*notificationv1.CreateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Create")
}
func (s *notifClientStub) List(_ context.Context, in *notificationv1.ListRequest, _ ...grpc.CallOption) (*notificationv1.ListResponse, error) {
	s.listReq = in
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listOut != nil {
		return s.listOut, nil
	}
	return &notificationv1.ListResponse{}, nil
}
func (s *notifClientStub) GetUnreadCount(context.Context, *notificationv1.GetUnreadCountRequest, ...grpc.CallOption) (*notificationv1.GetUnreadCountResponse, error) {
	return &notificationv1.GetUnreadCountResponse{UnreadCount: s.unreadOut}, nil
}
func (s *notifClientStub) MarkRead(_ context.Context, _ *notificationv1.MarkReadRequest, _ ...grpc.CallOption) (*notificationv1.MarkReadResponse, error) {
	if s.markErr != nil {
		return nil, s.markErr
	}
	return &notificationv1.MarkReadResponse{UnreadCount: s.markOut}, nil
}
func (s *notifClientStub) MarkAllRead(context.Context, *notificationv1.MarkAllReadRequest, ...grpc.CallOption) (*notificationv1.MarkAllReadResponse, error) {
	if s.markErr != nil {
		return nil, s.markErr
	}
	return &notificationv1.MarkAllReadResponse{UnreadCount: s.markOut}, nil
}

func newNotifHandler(c notificationGatewayClient) *NotificationHandler {
	return NewNotificationHandler(zerolog.Nop(), c, nil, nil)
}

func authedReq(t *testing.T, method, target, userID string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	if userID != "" {
		r = r.WithContext(middleware.WithUserID(r.Context(), userID))
	}
	return r
}

func TestNotifications_List_RequiresAuth(t *testing.T) {
	c := &notifClientStub{}
	h := newNotifHandler(c)
	rr := httptest.NewRecorder()
	h.List(rr, authedReq(t, http.MethodGet, "/notifications", ""))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
	if c.listReq != nil {
		t.Fatal("backend List must not be called for an unauthenticated request")
	}
}

func TestNotifications_List_ForwardsPagingAndFilter(t *testing.T) {
	c := &notifClientStub{listOut: &notificationv1.ListResponse{
		Notifications: []*notificationv1.Notification{{Id: "n1", Type: "x"}},
		Total:         1,
		UnreadCount:   1,
	}}
	h := newNotifHandler(c)
	rr := httptest.NewRecorder()
	h.List(rr, authedReq(t, http.MethodGet, "/notifications?limit=10&offset=5&unread_only=true", "user-1"))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if c.listReq.GetUserId() != "user-1" {
		t.Fatalf("user not forwarded: %s", c.listReq.GetUserId())
	}
	if c.listReq.GetLimit() != 10 || c.listReq.GetOffset() != 5 {
		t.Fatalf("paging not forwarded: limit=%d offset=%d", c.listReq.GetLimit(), c.listReq.GetOffset())
	}
	if !c.listReq.GetUnreadOnly() {
		t.Fatal("unread_only=true should be forwarded")
	}
	var body struct {
		Notifications []map[string]any `json:"notifications"`
		Total         int              `json:"total"`
		UnreadCount   int              `json:"unread_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Notifications) != 1 || body.Total != 1 || body.UnreadCount != 1 {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestNotifications_List_DefaultsPagingOnGarbage(t *testing.T) {
	c := &notifClientStub{}
	h := newNotifHandler(c)
	rr := httptest.NewRecorder()
	h.List(rr, authedReq(t, http.MethodGet, "/notifications?limit=abc&offset=-3", "user-1"))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	// parsePositiveInt: garbage/negative → default (limit 30, offset 0).
	if c.listReq.GetLimit() != 30 || c.listReq.GetOffset() != 0 {
		t.Fatalf("defaults not applied: limit=%d offset=%d", c.listReq.GetLimit(), c.listReq.GetOffset())
	}
}

func TestNotifications_List_ClampsLimitToMax(t *testing.T) {
	c := &notifClientStub{}
	h := newNotifHandler(c)
	rr := httptest.NewRecorder()
	h.List(rr, authedReq(t, http.MethodGet, "/notifications?limit=99999", "user-1"))

	if c.listReq.GetLimit() != 100 {
		t.Fatalf("limit should clamp to 100, got %d", c.listReq.GetLimit())
	}
}

func TestNotifications_List_BackendError(t *testing.T) {
	c := &notifClientStub{listErr: status.Error(codes.Internal, "boom")}
	h := newNotifHandler(c)
	rr := httptest.NewRecorder()
	h.List(rr, authedReq(t, http.MethodGet, "/notifications", "user-1"))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rr.Code)
	}
}

func TestNotifications_UnreadCount(t *testing.T) {
	c := &notifClientStub{unreadOut: 7}
	h := newNotifHandler(c)
	rr := httptest.NewRecorder()
	h.UnreadCount(rr, authedReq(t, http.MethodGet, "/notifications/unread-count", "user-1"))

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var body struct {
		UnreadCount int `json:"unread_count"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body.UnreadCount != 7 {
		t.Fatalf("unread_count = %d, want 7", body.UnreadCount)
	}
}

func TestNotifications_UnreadCount_RequiresAuth(t *testing.T) {
	h := newNotifHandler(&notifClientStub{})
	rr := httptest.NewRecorder()
	h.UnreadCount(rr, authedReq(t, http.MethodGet, "/notifications/unread-count", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestNotifications_MarkRead(t *testing.T) {
	c := &notifClientStub{markOut: 2}
	h := newNotifHandler(c)

	r := authedReq(t, http.MethodPost, "/notifications/n1/read", "user-1")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("notificationId", "n1")
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.MarkRead(rr, r)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func TestNotifications_MarkRead_RequiresAuth(t *testing.T) {
	h := newNotifHandler(&notifClientStub{})
	rr := httptest.NewRecorder()
	h.MarkRead(rr, authedReq(t, http.MethodPost, "/notifications/n1/read", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestNotifications_MarkAllRead(t *testing.T) {
	c := &notifClientStub{markOut: 0}
	h := newNotifHandler(c)
	rr := httptest.NewRecorder()
	h.MarkAllRead(rr, authedReq(t, http.MethodPost, "/notifications/read-all", "user-1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
}

func TestNotifications_MarkAllRead_RequiresAuth(t *testing.T) {
	h := newNotifHandler(&notifClientStub{})
	rr := httptest.NewRecorder()
	h.MarkAllRead(rr, authedReq(t, http.MethodPost, "/notifications/read-all", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}
