package grpcutil

// CallerID propagation for internal gRPC calls.
//
// The api-gateway verifies the JWT (ES256) and extracts the authenticated
// user_id from the token claims. It then injects that user_id into every
// outgoing gRPC call via the "x-caller-id" metadata key using
// CallerIDClientInterceptor.
//
// On the receiving service (e.g. booking-service), CallerIDServerInterceptor
// reads the key from incoming metadata and stores it in the request context.
// Handlers call CallerIDFromCtx(ctx) instead of reading user_id from proto
// fields — this ensures the identity is always the gateway-verified one, not
// whatever the caller chose to put in the request payload.
//
// Security model:
//   - Within the cluster (all traffic flows through api-gateway) this gives a
//     correct verified identity on every request.
//   - If a service is reachable directly (debug port, NetworkPolicy gap), the
//     metadata key will be absent or arbitrary. The server interceptor stores
//     whatever value arrives — it does NOT verify a signature. The defence
//     against direct access is NetworkPolicy / mTLS, not this interceptor.
//   - This is the same trust model used by "x-service-token": lightweight
//     defence-in-depth, not a cryptographic boundary.
//
// Usage (client — api-gateway, deps.go):
//
//	bookingConn, _ := mustDial(cfg.BookingAddr, "booking-service",
//	    grpc.WithUnaryInterceptor(grpcutil.CallerIDClientInterceptor(middleware.UserIDFromCtx)),
//	)
//
// Usage (server — booking-service, cmd/main.go):
//
//	grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcutil.CallerIDServerInterceptor(),
//	    // ... other interceptors
//	))
//
// Usage (handler):
//
//	callerID := grpcutil.CallerIDFromCtx(ctx)

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const CallerIDHeader = "x-caller-id"

type (
	ctxCallerIDKey struct{}
)

var callerIDKey = ctxCallerIDKey{}

// CallerIDFromCtx returns the caller user_id stored by CallerIDServerInterceptor.
// Returns empty string if the interceptor was not installed or the client did
// not send the header.
func CallerIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(callerIDKey).(string); ok {
		return v
	}
	return ""
}

// CallerIDClientInterceptor returns a gRPC unary client interceptor that reads
// the authenticated user_id from the outgoing request context using userIDFromCtx
// and injects it into the gRPC metadata as "x-caller-id".
//
// userIDFromCtx should be middleware.UserIDFromCtx (or any func that extracts
// the verified user identity from a context populated by the auth middleware).
// When it returns an empty string the header is not sent.
func CallerIDClientInterceptor(userIDFromCtx func(context.Context) string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if uid := strings.TrimSpace(userIDFromCtx(ctx)); uid != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, CallerIDHeader, uid)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// IsServiceCallerID reports whether the raw x-caller-id value belongs to an
// internal service rather than an end-user.
//
// Convention: end-user caller IDs are UUID v4 strings (injected by api-gateway
// from the verified JWT). Internal services inject a human-readable name such
// as "review-service" via CallerIDClientInterceptor. Anything that is not a
// valid UUID is treated as a service identity.
//
// This allows handlers to branch on caller type without hardcoding service
// names: if IsServiceCallerID(raw) { /* inter-service call */ }.
func IsServiceCallerID(raw string) bool {
	if raw == "" {
		// No caller ID present at all — treat as end-user (most restrictive path).
		return false
	}
	_, err := uuid.Parse(raw)
	return err != nil
}

// CallerIDServerInterceptor returns a gRPC unary server interceptor that reads
// "x-caller-id" from incoming metadata and stores it in the request context.
// Downstream handlers retrieve it with CallerIDFromCtx.
func CallerIDServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(CallerIDHeader); len(vals) > 0 {
				if uid := strings.TrimSpace(vals[0]); uid != "" {
					ctx = context.WithValue(ctx, callerIDKey, uid)
				}
			}
		}
		return handler(ctx, req)
	}
}
