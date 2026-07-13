package metrics

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// hijackableRecorder имитирует боевой ResponseWriter, поддерживающий Hijack.
type hijackableRecorder struct{ *httptest.ResponseRecorder }

func (h *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

// Регрессия: statusRecorder обязан пробрасывать http.Hijacker, иначе gorilla
// отвечает 500 на каждый WebSocket-апгрейд при включённых метриках.
func TestHTTPMiddleware_preservesHijacker(t *testing.T) {
	t.Parallel()

	var sawHijacker bool
	h := HTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, sawHijacker = w.(http.Hijacker)
	}))
	h.ServeHTTP(&hijackableRecorder{httptest.NewRecorder()},
		httptest.NewRequest(http.MethodGet, "/api/v2/chat/ws", http.NoBody))

	if !sawHijacker {
		t.Fatal("statusRecorder must forward http.Hijacker — WebSocket upgrades return 500 otherwise")
	}
}

// Регрессия: hijacked-соединение (WebSocket) считается как запрос, но его
// длительность (вся сессия — минуты/часы) НЕ попадает в гистограмму латентности,
// иначе p95 SLO уезжает в +Inf.
func TestHTTPMiddleware_hijackSkipsDuration(t *testing.T) {
	const route = "/api/v1/hijacktest"

	r := chi.NewRouter()
	r.Use(HTTPMiddleware)
	r.Get(route, func(w http.ResponseWriter, _ *http.Request) {
		_, _, _ = w.(http.Hijacker).Hijack() // имитируем WS-апгрейд
	})
	r.ServeHTTP(&hijackableRecorder{httptest.NewRecorder()},
		httptest.NewRequest(http.MethodGet, route, http.NoBody))

	c, err := httpRequests.GetMetricWithLabelValues(http.MethodGet, "2xx", route)
	if err != nil {
		t.Fatal(err)
	}
	if got := int(testutil.ToFloat64(c)); got != 1 {
		t.Fatalf("requests=%d want 1 (request must still be counted)", got)
	}

	if n := durationSampleCount(t, route); n != 0 {
		t.Fatalf("duration samples=%d want 0 (WS session must not pollute latency histogram)", n)
	}
}

// durationSampleCount returns the histogram sample count for httpDuration with
// the given route label, gathering the real registry.
func durationSampleCount(t *testing.T, route string) uint64 {
	t.Helper()
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "api_gateway_http_request_duration_seconds" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "route" && l.GetValue() == route {
					return m.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}

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

func TestHTTPMiddleware_unmatchedRouteClampsLabel(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Use(HTTPMiddleware)
	r.Get("/api/v1/ping", func(w http.ResponseWriter, _ *http.Request) {})

	// Scanner-style paths must all collapse into route="unmatched" —
	// raw URLs as label values would grow series without bound.
	for _, path := range []string{"/wp-admin/setup.php", "/.env", "/api/v2/unknown"} {
		req := httptest.NewRequest(http.MethodGet, path, http.NoBody)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	c, err := httpRequests.GetMetricWithLabelValues(http.MethodGet, "4xx", "unmatched")
	if err != nil {
		t.Fatal(err)
	}
	if got := int(testutil.ToFloat64(c)); got != 3 {
		t.Fatalf("unmatched requests=%d want 3", got)
	}
}

func TestObserveSupportWebhookDelivery(t *testing.T) {
	t.Parallel()

	beforeSuccess, err := supportWebhookDeliveries.GetMetricWithLabelValues("success")
	if err != nil {
		t.Fatal(err)
	}
	beforeError, err := supportWebhookDeliveries.GetMetricWithLabelValues("error")
	if err != nil {
		t.Fatal(err)
	}
	successStart := testutil.ToFloat64(beforeSuccess)
	errorStart := testutil.ToFloat64(beforeError)

	ObserveSupportWebhookDelivery("success")
	ObserveSupportWebhookDelivery("error")

	if got := testutil.ToFloat64(beforeSuccess); got != successStart+1 {
		t.Fatalf("success deliveries=%v want %v", got, successStart+1)
	}
	if got := testutil.ToFloat64(beforeError); got != errorStart+1 {
		t.Fatalf("error deliveries=%v want %v", got, errorStart+1)
	}
}
