package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "api_gateway"

var (
	registry = prometheus.NewRegistry()

	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "HTTP requests processed by api-gateway (route = chi route pattern when known).",
		},
		[]string{"method", "status_class", "route"},
	)

	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds (route = chi route pattern when known).",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "status_class", "route"},
	)
)

func init() {
	registry.MustRegister(httpRequests, httpDuration)
}

// Registry returns the dedicated prometheus registry (not global).
func Registry() *prometheus.Registry {
	return registry
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func statusClass(code int) string {
	if code == 0 {
		code = http.StatusOK
	}
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

func routeLabel(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := strings.TrimSpace(rc.RoutePattern()); p != "" {
			return p
		}
	}
	if p := strings.TrimSpace(r.URL.Path); p != "" {
		return p
	}
	return "/"
}

// HTTPMiddleware records RED-style metrics (rate, errors by class, duration).
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		sc := statusClass(sr.status)
		route := routeLabel(r)
		httpRequests.WithLabelValues(r.Method, sc, route).Inc()
		httpDuration.WithLabelValues(r.Method, sc, route).Observe(time.Since(start).Seconds())
	})
}
