package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestCollectEvent_InvalidJSON(t *testing.T) {
	h := NewAnalyticsHandler(zerolog.Nop(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/events", bytes.NewBufferString("not-json"))
	rr := httptest.NewRecorder()
	h.CollectEvent(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestCollectEvent_InvalidName(t *testing.T) {
	h := NewAnalyticsHandler(zerolog.Nop(), nil)
	body := `{"name":"BadName","props":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/events", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CollectEvent(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("code=%d", rr.Code)
	}
}

func TestCollectEvent_OK(t *testing.T) {
	h := NewAnalyticsHandler(zerolog.Nop(), nil)
	body := `{"name":"page_view","props":{"path":"/venues"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/events", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CollectEvent(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}
