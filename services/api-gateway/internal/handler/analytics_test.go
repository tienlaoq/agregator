package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// filterProps unit tests ------------------------------------------------

func TestFilterProps_allowsWhitelistedKeys(t *testing.T) {
	in := map[string]any{
		"page":           "/venues",
		"path":           "/venues",
		"referrer":       "https://google.com",
		"event_category": "booking",
		"event_label":    "confirm",
		"value":          42,
		"locale":         "ru",
		"screen":         "1920x1080",
		"duration_ms":    1234,
	}
	out := filterProps(in)
	assert.Equal(t, len(in), len(out), "all whitelisted keys must pass through")
	for k := range in {
		assert.Contains(t, out, k)
	}
}

func TestFilterProps_allowsUtmKeys(t *testing.T) {
	in := map[string]any{
		"utm_source":   "google",
		"utm_medium":   "cpc",
		"utm_campaign": "banya_summer",
		"utm_term":     "баня москва",
		"utm_content":  "hero_banner",
	}
	out := filterProps(in)
	assert.Equal(t, len(in), len(out), "all utm_* keys must pass through")
}

func TestFilterProps_dropsPIIKeys(t *testing.T) {
	in := map[string]any{
		"email":       "user@example.com",
		"phone":       "+79991234567",
		"user_id":     "abc-123",
		"name":        "Иван Иванов",
		"page":        "/venues", // whitelisted — must survive
		"utm_source":  "fb",      // utm — must survive
	}
	out := filterProps(in)

	// PII keys must be absent.
	assert.NotContains(t, out, "email")
	assert.NotContains(t, out, "phone")
	assert.NotContains(t, out, "user_id")
	assert.NotContains(t, out, "name")

	// Whitelisted keys must remain.
	require.Contains(t, out, "page")
	require.Contains(t, out, "utm_source")
	assert.Equal(t, 2, len(out))
}

func TestFilterProps_dropsUnknownKeys(t *testing.T) {
	in := map[string]any{
		"totally_custom_field": "whatever",
		"another_random_key":   123,
		"path":                 "/checkout", // whitelisted
	}
	out := filterProps(in)
	assert.NotContains(t, out, "totally_custom_field")
	assert.NotContains(t, out, "another_random_key")
	assert.Contains(t, out, "path")
	assert.Equal(t, 1, len(out))
}

func TestFilterProps_emptyProps(t *testing.T) {
	out := filterProps(map[string]any{})
	assert.Empty(t, out)
}

// CollectEvent integration — PII in props must not reach the handler output.
func TestCollectEvent_PIIPropsDropped(t *testing.T) {
	// We can't inspect the zerolog output easily here, but we can verify the
	// handler still returns 204 (not a validation error) — the PII keys are
	// silently dropped, not rejected. This matches the silent-drop contract.
	h := NewAnalyticsHandler(zerolog.Nop(), nil)
	body := `{"name":"checkout_start","props":{"email":"user@example.com","path":"/checkout"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analytics/events", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CollectEvent(rr, req)
	assert.Equal(t, http.StatusNoContent, rr.Code, "PII keys should be dropped silently, not rejected")
}
