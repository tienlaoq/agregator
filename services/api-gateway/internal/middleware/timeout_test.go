package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRequestTimeout_FastHandler_OK(t *testing.T) {
	h := RequestTimeout(100 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestTimeout_SlowHandler_503(t *testing.T) {
	h := RequestTimeout(20 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a handler that blocks until context is cancelled.
		<-r.Context().Done()
		// http.TimeoutHandler writes 503 itself; this write is a no-op.
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "request timeout")
}

func TestRequestTimeout_ContextCancelledPropagated(t *testing.T) {
	// Verify that the context passed to the handler is cancelled when the
	// timeout fires, so downstream gRPC calls also get cancelled.
	//
	// http.TimeoutHandler returns from ServeHTTP as soon as its own timer
	// fires, which races the handler goroutine that observes ctx.Done() and
	// records ctxErr. Wait for the handler goroutine to finish before
	// asserting, otherwise the read of ctxErr is non-deterministic.
	var ctxErr error
	handlerDone := make(chan struct{})
	h := RequestTimeout(20 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		ctxErr = r.Context().Err()
		close(handlerDone)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not observe context cancellation within 1s")
	}

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.NotNil(t, ctxErr, "handler context should be cancelled after timeout")
}
