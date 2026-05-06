package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
)

func TestSupportContactSuccess(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer token-123" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode helpdesk payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	h := NewSupportHandler(zerolog.Nop(), srv.URL, "token-123", []string{"mod1@example.com", "mod2@example.com"}, nil)

	body := []byte(`{"topic":"Оплата","message":"Не прошел платеж","booking_id":"b1","payment_id":"p1","source_page":"/my/bookings"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/support/contact", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.CtxUserID, "u-1")
	ctx = context.WithValue(ctx, middleware.CtxRole, "user")
	ctx = context.WithValue(ctx, middleware.CtxEmail, "user@example.com")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.Contact(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	ticketNumber, _ := out["ticket_number"].(string)
	if !strings.HasPrefix(ticketNumber, "SUP-") || len(ticketNumber) != 16 {
		t.Fatalf("unexpected ticket_number=%q", ticketNumber)
	}
	if got["topic"] != "Оплата" || got["message"] != "Не прошел платеж" {
		t.Fatalf("unexpected payload fields: %+v", got)
	}
	if got["user_id"] != "u-1" || got["role"] != "user" {
		t.Fatalf("missing user context in payload: %+v", got)
	}
	if got["email"] != "user@example.com" {
		t.Fatalf("expected email from context, got=%v", got["email"])
	}
	if got["request_id"] == "" {
		t.Fatalf("request_id must be present in payload: %+v", got)
	}
	targetRoles, ok := got["target_roles"].([]any)
	if !ok || len(targetRoles) != 1 || targetRoles[0] != "admin" {
		t.Fatalf("target_roles must include admin: %+v", got)
	}
	recipients, ok := got["recipient_emails"].([]any)
	if !ok || len(recipients) != 2 {
		t.Fatalf("recipient_emails must include moderators: %+v", got)
	}
}

func TestSupportContactValidation(t *testing.T) {
	h := NewSupportHandler(zerolog.Nop(), "http://example.local", "", nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/support/contact", bytes.NewReader([]byte(`{"topic":" ","message":""}`)))
	rec := httptest.NewRecorder()
	h.Contact(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSupportContactWebhookFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	h := NewSupportHandler(zerolog.Nop(), srv.URL, "", nil, nil)
	body := []byte(`{"topic":"Оплата","message":"Ошибка"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/support/contact", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Contact(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSupportAdminReplyValidation(t *testing.T) {
	h := NewSupportHandler(zerolog.Nop(), "", "", nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/support/reply", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	h.AdminReply(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty body: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/support/reply", bytes.NewReader([]byte(
		`{"user_email":"a@b.ru","reply":"ok","ticket_number":"SUP-1","request_id":"not-a-uuid"}`,
	)))
	rec = httptest.NewRecorder()
	h.AdminReply(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid uuid: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSupportAdminReplySMTPRequired(t *testing.T) {
	h := NewSupportHandler(zerolog.Nop(), "", "", nil, nil)
	body := []byte(`{"user_email":"user@example.com","reply":"Привет","ticket_number":"SUP-123456789ABC","request_id":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/support/reply", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.AdminReply(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 when SMTP off, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}
