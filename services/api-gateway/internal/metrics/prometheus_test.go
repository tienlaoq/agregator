package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPMiddleware_routeAndStatus(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Use(HTTPMiddleware)
	r.Get("/api/v1/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", http.NoBody)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Fatalf("status=%d", rr.Code)
	}

	c, err := httpRequests.GetMetricWithLabelValues(http.MethodGet, "4xx", "/api/v1/ping")
	if err != nil {
		t.Fatal(err)
	}
	if got := int(testutil.ToFloat64(c)); got != 1 {
		t.Fatalf("requests=%d want 1", got)
	}
}
