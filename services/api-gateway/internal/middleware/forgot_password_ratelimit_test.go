package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForgotPasswordRateLimit_allowsUnderMax(t *testing.T) {
	t.Setenv("FORGOT_PASSWORD_RATE_LIMIT_MAX", "3")
	t.Setenv("FORGOT_PASSWORD_RATE_LIMIT_WINDOW", "1h")

	var hits int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ })
	h := ForgotPasswordRateLimit(zerolog.Nop())(next)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", nil)
		req.RemoteAddr = "192.168.1.10:12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 3, hits)
}

func TestForgotPasswordRateLimit_blocksAtMax(t *testing.T) {
	t.Setenv("FORGOT_PASSWORD_RATE_LIMIT_MAX", "2")
	t.Setenv("FORGOT_PASSWORD_RATE_LIMIT_WINDOW", "1h")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := ForgotPasswordRateLimit(zerolog.Nop())(next)

	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", nil)
		r.RemoteAddr = "10.0.0.5:9999"
		return r
	}

	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req())
	assert.Equal(t, http.StatusOK, rec1.Code)

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req())
	assert.Equal(t, http.StatusOK, rec2.Code)

	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req())
	assert.Equal(t, http.StatusTooManyRequests, rec3.Code)
}

func TestForgotPasswordRateLimit_slidingWindow(t *testing.T) {
	t.Setenv("FORGOT_PASSWORD_RATE_LIMIT_MAX", "1")
	t.Setenv("FORGOT_PASSWORD_RATE_LIMIT_WINDOW", "50ms")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := ForgotPasswordRateLimit(zerolog.Nop())(next)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", nil)
	r.RemoteAddr = "172.16.0.1:1"

	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, r)
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, r)
	require.Equal(t, http.StatusTooManyRequests, rec2.Code)

	time.Sleep(60 * time.Millisecond)

	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, r)
	assert.Equal(t, http.StatusOK, rec3.Code)
}

func TestForgotPasswordRateLimit_disabledWhenMaxZero(t *testing.T) {
	t.Setenv("FORGOT_PASSWORD_RATE_LIMIT_MAX", "0")

	var n int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n++ })
	h := ForgotPasswordRateLimit(zerolog.Nop())(next)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", nil)
	r.RemoteAddr = "1.2.3.4:1"
	for i := 0; i < 5; i++ {
		h.ServeHTTP(httptest.NewRecorder(), r)
	}
	assert.Equal(t, 5, n)
}
