package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func Test_shortMethod(t *testing.T) {
	tests := []struct {
		name string
		full string
		want string
	}{
		{"standard", "/booking.v1.BookingService/CreateBooking", "BookingService/CreateBooking"},
		{"no package", "/BookingService/CreateBooking", "BookingService/CreateBooking"},
		{"no leading slash", "booking.v1.BookingService/CreateBooking", "BookingService/CreateBooking"},
		{"malformed", "garbage", "garbage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortMethod(tt.full); got != tt.want {
				t.Errorf("shortMethod(%q) = %q, want %q", tt.full, got, tt.want)
			}
		})
	}
}

func Test_UnaryServerInterceptor_recordsCodeAndMethod(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"ok", nil, "OK"},
		{"not found", status.Error(codes.NotFound, "nope"), "NotFound"},
		{"plain error maps to Unknown", context.DeadlineExceeded, "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("test-service")
			ic := m.UnaryServerInterceptor()
			info := &grpc.UnaryServerInfo{FullMethod: "/booking.v1.BookingService/CreateBooking"}
			_, _ = ic(context.Background(), nil, info, func(context.Context, any) (any, error) {
				return nil, tt.err
			})

			got := testutil.ToFloat64(m.grpcHandled.WithLabelValues("BookingService/CreateBooking", tt.wantCode))
			if got != 1 {
				t.Errorf("grpc_handled_total{code=%q} = %v, want 1", tt.wantCode, got)
			}
		})
	}
}

func Test_ObserveNATS_clampsUnknownResult(t *testing.T) {
	m := New("test-service")
	m.ObserveNATS("payment.completed", "weird", 10*time.Millisecond)
	m.ObserveNATS("payment.completed", NATSOk, 10*time.Millisecond)

	if got := testutil.ToFloat64(m.natsMessages.WithLabelValues("payment.completed", NATSError)); got != 1 {
		t.Errorf("unknown result not clamped to error: got %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.natsMessages.WithLabelValues("payment.completed", NATSOk)); got != 1 {
		t.Errorf("ok result: got %v, want 1", got)
	}
}

func Test_handler_endpoints(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		ready      func(context.Context) error
		wantStatus int
	}{
		{"healthz always ok", "/healthz", nil, http.StatusOK},
		{"readyz nil ready", "/readyz", nil, http.StatusOK},
		{"readyz ok", "/readyz", func(context.Context) error { return nil }, http.StatusOK},
		{"readyz failing", "/readyz", func(context.Context) error { return context.Canceled }, http.StatusServiceUnavailable},
		{"metrics exposition", "/metrics", nil, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("test-service")
			rec := httptest.NewRecorder()
			handler(m.registry, tt.ready).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.wantStatus)
			}
		})
	}
}

func Test_handler_metricsNamespace(t *testing.T) {
	m := New("test-service")
	m.ObserveNATS("review.created", NATSOk, time.Millisecond)

	rec := httptest.NewRecorder()
	handler(m.registry, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "test_service_nats_messages_total") {
		t.Errorf("exposition missing namespaced metric, body:\n%s", body[:min(len(body), 500)])
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Error("exposition missing go runtime collector")
	}
}
