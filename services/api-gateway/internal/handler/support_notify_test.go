package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"github.com/tienlao/agregator/services/api-gateway/internal/supportstore"
)

type stubSupportTicketsRepo struct {
	insertedID uuid.UUID
	notifies   []string
}

func (s *stubSupportTicketsRepo) Insert(ctx context.Context, p supportstore.InsertParams) error {
	s.insertedID = p.RequestID
	return nil
}

func (s *stubSupportTicketsRepo) SetNotifyStatus(ctx context.Context, requestID uuid.UUID, status string) error {
	if requestID != s.insertedID {
		panic("notify for unexpected id")
	}
	s.notifies = append(s.notifies, status)
	return nil
}

func (s *stubSupportTicketsRepo) List(ctx context.Context, limit, offset int) ([]supportstore.Row, int64, error) {
	return nil, 0, nil
}

func (s *stubSupportTicketsRepo) GetByRequestID(ctx context.Context, id uuid.UUID) (*supportstore.Row, error) {
	return nil, supportstore.ErrNotFound
}

func (s *stubSupportTicketsRepo) GetByTicketNumber(ctx context.Context, ticketNumber string) (*supportstore.Row, error) {
	return nil, supportstore.ErrNotFound
}

func (s *stubSupportTicketsRepo) MarkReplied(ctx context.Context, id uuid.UUID, moderatorID string) error {
	return nil
}

func TestSupportContactWebhookFailureSetsNotifyFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	repo := &stubSupportTicketsRepo{}
	h := NewSupportHandler(zerolog.Nop(), srv.URL, "", nil, repo)

	body := []byte(`{"topic":"Оплата","message":"Ошибка"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/support/contact", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Contact(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(repo.notifies) != 1 || repo.notifies[0] != supportstore.NotifyFailed {
		t.Fatalf("want single notify_status failed, got %+v", repo.notifies)
	}
}

func TestSupportContactWebhookSuccessSetsNotifyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	repo := &stubSupportTicketsRepo{}
	h := NewSupportHandler(zerolog.Nop(), srv.URL, "", nil, repo)

	body := []byte(`{"topic":"T","message":"M"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/support/contact", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Contact(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(repo.notifies) != 1 || repo.notifies[0] != supportstore.NotifyOK {
		t.Fatalf("want notify ok, got %+v", repo.notifies)
	}
}

func TestSupportContactInboxOnlySetsNotifyOK(t *testing.T) {
	repo := &stubSupportTicketsRepo{}
	h := NewSupportHandler(zerolog.Nop(), "", "", nil, repo)

	body := []byte(`{"topic":"T","message":"M"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/support/contact", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.CtxUserID, "u-1")
	ctx = context.WithValue(ctx, middleware.CtxRole, "user")
	ctx = context.WithValue(ctx, middleware.CtxEmail, "x@y.z")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.Contact(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(repo.notifies) != 1 || repo.notifies[0] != supportstore.NotifyOK {
		t.Fatalf("want inbox-only notify ok, got %+v", repo.notifies)
	}
}
