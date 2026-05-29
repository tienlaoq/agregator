package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func panicHandler(msg string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(msg)
	})
}

func TestRecoverer_returns500OnPanic(t *testing.T) {
	h := Recoverer(zerolog.Nop())(panicHandler("something broke"))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestRecoverer_bodyContainsNoPanicMessage(t *testing.T) {
	h := Recoverer(zerolog.Nop())(panicHandler("super secret internal detail"))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	assert.NotContains(t, body, "super secret internal detail",
		"panic message must not be exposed in the HTTP response body")
	assert.NotContains(t, body, "goroutine",
		"stack trace must not be exposed in the HTTP response body")
}

func TestRecoverer_bodyIsGenericCatalogError(t *testing.T) {
	h := Recoverer(zerolog.Nop())(panicHandler("boom"))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "GATEWAY.UPSTREAM.INTERNAL",
		"response must contain the catalog error code")
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func TestRecoverer_logsPanicAndStack(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf)

	h := Recoverer(log)(panicHandler("disk on fire"))

	req := httptest.NewRequest(http.MethodGet, "/explode", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "disk on fire", "panic value must appear in log")
	assert.Contains(t, logOutput, "panic recovered", "log message must be 'panic recovered'")
	assert.Contains(t, logOutput, "stack", "stack trace must be present in log")
	assert.Contains(t, logOutput, "/explode", "request path must be in log")
}

func TestRecoverer_includesRequestIDInLog(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf)

	h := Recoverer(log)(panicHandler("oops"))

	req := httptest.NewRequest(http.MethodPost, "/crash", nil)
	req = req.WithContext(WithRequestID(req.Context(), "req-test-123"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Contains(t, buf.String(), "req-test-123")
}

func TestRecoverer_passesThrough_whenNoPanic(t *testing.T) {
	h := Recoverer(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("all good"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.Contains(rec.Body.String(), "all good"))
}

func TestRecoverer_handlesNilPanic(t *testing.T) {
	h := Recoverer(zerolog.Nop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(nil) //nolint:gocritic // intentional nil panic for test coverage
	}))

	req := httptest.NewRequest(http.MethodGet, "/nil-panic", nil)
	rec := httptest.NewRecorder()

	// Must not crash the test process.
	require.NotPanics(t, func() {
		h.ServeHTTP(rec, req)
	})
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
