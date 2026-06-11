package metrics

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnaryServerInterceptor records RED metrics for every unary gRPC call.
// Place it FIRST in grpc.ChainUnaryInterceptor so it is outermost and observes
// the final status code after inner interceptors (e.g. PgErrorUnaryInterceptor)
// have mapped errors.
func (m *Metrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		method := shortMethod(info.FullMethod)
		m.grpcHandled.WithLabelValues(method, status.Code(err).String()).Inc()
		m.grpcDuration.WithLabelValues(method).Observe(time.Since(start).Seconds())
		return resp, err
	}
}

// shortMethod turns "/booking.v1.BookingService/CreateBooking" into
// "BookingService/CreateBooking" — unique within a service, without the
// proto package noise.
func shortMethod(full string) string {
	full = strings.TrimPrefix(full, "/")
	svc, method, ok := strings.Cut(full, "/")
	if !ok {
		return full
	}
	if i := strings.LastIndex(svc, "."); i >= 0 {
		svc = svc[i+1:]
	}
	return svc + "/" + method
}
