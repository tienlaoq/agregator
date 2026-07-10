package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	notificationv1 "github.com/tienlao/agregator/gen/go/notification/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/apicatalog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// notifCapture records every Create call so the Notify* builders can be asserted.
type notifCapture struct {
	notificationGatewayClient
	reqs      []*notificationv1.CreateRequest
	createErr error
}

func (c *notifCapture) Create(_ context.Context, in *notificationv1.CreateRequest, _ ...grpc.CallOption) (*notificationv1.CreateResponse, error) {
	c.reqs = append(c.reqs, in)
	if c.createErr != nil {
		return nil, c.createErr
	}
	return &notificationv1.CreateResponse{Notification: &notificationv1.Notification{Id: "n1", UserId: in.GetUserId()}}, nil
}

func (c *notifCapture) last() *notificationv1.CreateRequest {
	if len(c.reqs) == 0 {
		return nil
	}
	return c.reqs[len(c.reqs)-1]
}

// dataKind extracts the "kind" field from a Create request's JSON data payload.
func dataKind(t *testing.T, req *notificationv1.CreateRequest) string {
	t.Helper()
	if req == nil {
		t.Fatal("nil create request")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(req.GetData()), &m); err != nil {
		t.Fatalf("data not JSON: %v (%q)", err, req.GetData())
	}
	k, _ := m["kind"].(string)
	return k
}

func TestNotify_SkipsWhenNoClientOrUser(t *testing.T) {
	// nil client
	h := newNotifHandler(nil)
	h.Notify(context.Background(), "u1", "t", "T", "B", "") // must not panic

	// empty user id
	cap := &notifCapture{}
	h2 := newNotifHandler(cap)
	h2.Notify(context.Background(), "  ", "t", "T", "B", "")
	if len(cap.reqs) != 0 {
		t.Fatalf("expected no create for blank user, got %d", len(cap.reqs))
	}
}

func TestNotify_CreateErrorSwallowed(t *testing.T) {
	cap := &notifCapture{createErr: status.Error(codes.Internal, "boom")}
	h := newNotifHandler(cap)
	h.Notify(context.Background(), "u1", "t", "T", "B", "") // logged, never panics
	if len(cap.reqs) != 1 {
		t.Fatalf("create should still be attempted once, got %d", len(cap.reqs))
	}
}

func TestNotifyStaffInvited(t *testing.T) {
	cap := &notifCapture{}
	h := newNotifHandler(cap)
	h.NotifyStaffInvited(context.Background(), "u1", "v1", "manager")
	req := cap.last()
	if req.GetType() != "crm_staff_invited" || req.GetUserId() != "u1" {
		t.Fatalf("req = %+v", req)
	}
	if dataKind(t, req) != "crm_staff_invited" {
		t.Fatalf("kind = %s", dataKind(t, req))
	}
	// manager role produces the "менеджер" label in the body
	if !contains(req.GetBody(), "менеджер") {
		t.Fatalf("body missing role label: %s", req.GetBody())
	}
}

func TestNotifyTaskAssigned_WithAndWithoutTitle(t *testing.T) {
	cap := &notifCapture{}
	h := newNotifHandler(cap)

	h.NotifyTaskAssigned(context.Background(), "a1", "v1", "t1", "Clean")
	if !contains(cap.last().GetBody(), "«Clean»") {
		t.Fatalf("titled body = %s", cap.last().GetBody())
	}

	h.NotifyTaskAssigned(context.Background(), "a1", "v1", "t1", "  ")
	if contains(cap.last().GetBody(), "«") {
		t.Fatalf("untitled body should not embed quotes: %s", cap.last().GetBody())
	}
}

func TestNotifyVenueBookingCreated(t *testing.T) {
	cap := &notifCapture{}
	h := newNotifHandler(cap)
	h.NotifyVenueBookingCreated(context.Background(), "owner", "v1", "Баня", "b1", "2026-01-02", "10:00", "12:00", 3)
	req := cap.last()
	if req.GetType() != "venue_booking_created" || !contains(req.GetTitle(), "Баня") {
		t.Fatalf("req = %+v", req)
	}
	if !contains(req.GetBody(), "гостей: 3") {
		t.Fatalf("body missing guests: %s", req.GetBody())
	}
}

func TestNotifyVenueModerated_Actions(t *testing.T) {
	tests := []struct {
		action   string
		wantSent bool
	}{
		{"approve", true},
		{"reject", true},
		{"suspend", true},
		{"resume", true},
		{"unknown", false},
	}
	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			cap := &notifCapture{}
			h := newNotifHandler(cap)
			h.NotifyVenueModerated(context.Background(), "owner", "v1", "Баня", tc.action, "reason")
			if tc.wantSent && len(cap.reqs) != 1 {
				t.Fatalf("expected send for %s, got %d", tc.action, len(cap.reqs))
			}
			if !tc.wantSent && len(cap.reqs) != 0 {
				t.Fatalf("expected no send for %s, got %d", tc.action, len(cap.reqs))
			}
		})
	}
}

func TestNotifyVenueReviewCreated_RatingInBody(t *testing.T) {
	cap := &notifCapture{}
	h := newNotifHandler(cap)
	h.NotifyVenueReviewCreated(context.Background(), "owner", "v1", "Баня", 4)
	if !contains(cap.last().GetBody(), "Оценка 4 из 5") {
		t.Fatalf("body = %s", cap.last().GetBody())
	}
	// out-of-range rating falls back to the generic body
	cap2 := &notifCapture{}
	h2 := newNotifHandler(cap2)
	h2.NotifyVenueReviewCreated(context.Background(), "owner", "v1", "Баня", 0)
	if contains(cap2.last().GetBody(), "Оценка") {
		t.Fatalf("out-of-range rating should use generic body: %s", cap2.last().GetBody())
	}
}

func TestNotifyMasterBookingCreated_TitleWithName(t *testing.T) {
	cap := &notifCapture{}
	h := newNotifHandler(cap)
	h.NotifyMasterBookingCreated(context.Background(), "mu", "m1", "Иван", "b1", "2026-01-02", "10:00", "11:00")
	if !contains(cap.last().GetTitle(), "Иван") {
		t.Fatalf("title = %s", cap.last().GetTitle())
	}
	// empty name → generic title
	cap2 := &notifCapture{}
	h2 := newNotifHandler(cap2)
	h2.NotifyMasterBookingCreated(context.Background(), "mu", "m1", "", "b1", "2026-01-02", "", "")
	if cap2.last().GetTitle() != "Новая запись" {
		t.Fatalf("generic title = %s", cap2.last().GetTitle())
	}
}

func TestNotifyMasterModerated_Actions(t *testing.T) {
	for _, action := range []string{"approve", "reject", "suspend", "resume"} {
		cap := &notifCapture{}
		h := newNotifHandler(cap)
		h.NotifyMasterModerated(context.Background(), "mu", "m1", "Иван", action, "reason")
		if len(cap.reqs) != 1 {
			t.Fatalf("action %s: expected 1 send, got %d", action, len(cap.reqs))
		}
	}
	cap := &notifCapture{}
	h := newNotifHandler(cap)
	h.NotifyMasterModerated(context.Background(), "mu", "m1", "Иван", "bogus", "")
	if len(cap.reqs) != 0 {
		t.Fatalf("unknown action should not send, got %d", len(cap.reqs))
	}
}

func TestNotifyMasterReviewCreated(t *testing.T) {
	cap := &notifCapture{}
	h := newNotifHandler(cap)
	h.NotifyMasterReviewCreated(context.Background(), "mu", "m1", "Иван", 5)
	req := cap.last()
	if dataKind(t, req) != "master_review_created" || !contains(req.GetTitle(), "Иван") {
		t.Fatalf("req = %+v", req)
	}
}

// ── fan-out + ticket ──────────────────────────────────────────────────────────

func TestHandleFanoutMessage_IgnoresMalformed(t *testing.T) {
	h := newNotifHandler(&notifCapture{})
	// none of these should panic
	h.HandleFanoutMessage([]byte(`not json`))
	h.HandleFanoutMessage([]byte(`{"user_ids":[],"payload":{"a":1}}`))
	h.HandleFanoutMessage([]byte(`{"user_ids":["u1"],"payload":null}`))
	// valid: broadcasts to a hub with no sockets (no-op, must not panic)
	h.HandleFanoutMessage([]byte(`{"user_ids":["u1"],"payload":{"event":"x"}}`))
}

func TestEmitToUsers_NoNatsFallsBackToHub(t *testing.T) {
	h := newNotifHandler(&notifCapture{}) // natsConn nil
	h.emitToUsers([]string{"u1"}, map[string]any{"event": "x"})
}

func TestIssueWSTicket_RequiresAuth(t *testing.T) {
	h := newNotifHandler(&notifCapture{})
	rr := httptest.NewRecorder()
	h.IssueWSTicket(rr, httptest.NewRequest(http.MethodPost, "/ws-ticket", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestIssueWSTicket_NilRedisUnavailable(t *testing.T) {
	h := newNotifHandler(&notifCapture{}) // ticketRedis nil
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ws-ticket", nil)
	r = r.WithContext(middleware.WithUserID(r.Context(), "u1"))
	h.IssueWSTicket(rr, r)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestNotificationWS_RequiresAuth(t *testing.T) {
	h := newNotifHandler(&notifCapture{})
	rr := httptest.NewRecorder()
	h.WS(rr, httptest.NewRequest(http.MethodGet, "/ws", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rr.Code)
	}
	_ = apicatalog.GatewayAuthUnauthorized // referenced for clarity
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
