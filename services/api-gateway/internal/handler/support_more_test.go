package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/services/api-gateway/internal/supportstore"
)

// flexTicketsRepo is a configurable SupportTicketsPersistence for admin reads.
type flexTicketsRepo struct {
	stubSupportTicketsRepo
	rows    []supportstore.Row
	total   int64
	listErr error
	getRow  *supportstore.Row
	getErr  error
}

func (r *flexTicketsRepo) List(_ context.Context, _, _ int) ([]supportstore.Row, int64, error) {
	return r.rows, r.total, r.listErr
}

func (r *flexTicketsRepo) GetByRequestID(_ context.Context, _ uuid.UUID) (*supportstore.Row, error) {
	return r.getRow, r.getErr
}

func newSupportHandler(tickets SupportTicketsPersistence) *SupportHandler {
	return NewSupportHandler(zerolog.Nop(), "", "", nil, tickets)
}

func adminReq(method, target string, params map[string]string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	return r
}

func TestSupportAdminListTickets_NilStorage(t *testing.T) {
	h := newSupportHandler(nil)
	rr := httptest.NewRecorder()
	h.AdminListTickets(rr, adminReq(http.MethodGet, "/admin/support/tickets", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestSupportAdminListTickets_Success(t *testing.T) {
	repo := &flexTicketsRepo{
		total: 2,
		rows: []supportstore.Row{
			{RequestID: uuid.New(), TicketNumber: "SUP-ABC", Topic: "billing", CreatedAt: time.Now()},
		},
	}
	h := newSupportHandler(repo)
	rr := httptest.NewRecorder()
	h.AdminListTickets(rr, adminReq(http.MethodGet, "/admin/support/tickets?limit=10&offset=0", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Tickets []map[string]any `json:"tickets"`
		Total   int64            `json:"total"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Total != 2 || len(body.Tickets) != 1 {
		t.Fatalf("total=%d len=%d", body.Total, len(body.Tickets))
	}
}

func TestSupportAdminGetTicket_InvalidID(t *testing.T) {
	h := newSupportHandler(&flexTicketsRepo{})
	rr := httptest.NewRecorder()
	h.AdminGetTicket(rr, adminReq(http.MethodGet, "/admin/support/tickets/not-a-uuid", map[string]string{"requestID": "not-a-uuid"}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestSupportAdminGetTicket_NotFound(t *testing.T) {
	repo := &flexTicketsRepo{getErr: supportstore.ErrNotFound}
	h := newSupportHandler(repo)
	rr := httptest.NewRecorder()
	id := uuid.New().String()
	h.AdminGetTicket(rr, adminReq(http.MethodGet, "/admin/support/tickets/"+id, map[string]string{"requestID": id}))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestSupportAdminGetTicket_Success(t *testing.T) {
	rid := uuid.New()
	repo := &flexTicketsRepo{getRow: &supportstore.Row{RequestID: rid, TicketNumber: "SUP-XYZ", Topic: "help", CreatedAt: time.Now()}}
	h := newSupportHandler(repo)
	rr := httptest.NewRecorder()
	h.AdminGetTicket(rr, adminReq(http.MethodGet, "/admin/support/tickets/"+rid.String(), map[string]string{"requestID": rid.String()}))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body struct {
		Ticket map[string]any `json:"ticket"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body.Ticket["ticket_number"] != "SUP-XYZ" {
		t.Fatalf("ticket = %v", body.Ticket)
	}
}

// --- pure helpers ---

func TestSupportTicketToJSON_RepliedAtNilAndSet(t *testing.T) {
	nilRow := supportTicketToJSON(supportstore.Row{RequestID: uuid.New(), CreatedAt: time.Now()})
	if v, ok := nilRow["replied_at"]; !ok || v != nil {
		t.Fatalf("replied_at should be present and nil: %v", nilRow["replied_at"])
	}
	when := time.Now()
	setRow := supportTicketToJSON(supportstore.Row{RequestID: uuid.New(), CreatedAt: time.Now(), RepliedAt: &when})
	if setRow["replied_at"] == nil {
		t.Fatal("replied_at should be set")
	}
}

func TestClampString(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel"},
		{"", 5, ""},
		{"abc", 0, ""},
		{"abc", -1, ""},
		{"привет", 3, "при"}, // multibyte: clamp on rune boundary
	}
	for _, tc := range tests {
		if got := clampString(tc.s, tc.maxLen); got != tc.want {
			t.Fatalf("clampString(%q,%d) = %q, want %q", tc.s, tc.maxLen, got, tc.want)
		}
	}
}

func TestSupportReplyReferenceLabel(t *testing.T) {
	if got := supportReplyReferenceLabel("  sup-abc ", "rid"); got != "SUP-ABC" {
		t.Fatalf("got %q", got)
	}
	if got := supportReplyReferenceLabel("  ", "  rid-1 "); got != "rid-1" {
		t.Fatalf("fallback got %q", got)
	}
}

func TestUniqueEmails(t *testing.T) {
	got := uniqueEmails([]string{" A@x.io ", "a@x.io", "", "  ", "B@x.io"})
	want := []string{"a@x.io", "b@x.io"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestSupportTicketNumber(t *testing.T) {
	if got := supportTicketNumber("abcdef12-3456-7890-abcd-ef1234567890"); got != "SUP-ABCDEF123456" {
		t.Fatalf("got %q", got)
	}
	if got := supportTicketNumber("  "); got != "SUP-UNKNOWN" {
		t.Fatalf("empty got %q", got)
	}
	if got := supportTicketNumber("ab-cd"); got != "SUP-ABCD" {
		t.Fatalf("short got %q", got)
	}
}
