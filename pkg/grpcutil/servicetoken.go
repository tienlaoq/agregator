package grpcutil

// P0#6: service-to-service token authentication for internal gRPC.
//
// Without mTLS, any process that can reach chat-service over the network can
// call ListThreads/SendMessage/MarkRead with an arbitrary user_id and impersonate
// any user.  A shared service token stored in each service's environment and
// checked on the server side gives a lightweight defence layer until mTLS is
// rolled out.
//
// Usage (server side — chat-service):
//
//	token := os.Getenv("INTERNAL_SERVICE_TOKEN")
//	if token != "" {
//	    opts = append(opts, grpc.UnaryInterceptor(grpcutil.ServiceTokenServerInterceptor(token)))
//	}
//
// Usage (client side — api-gateway):
//
//	token := os.Getenv("INTERNAL_SERVICE_TOKEN")
//	if token != "" {
//	    chatDialOpts = append(chatDialOpts,
//	        grpc.WithUnaryInterceptor(grpcutil.ServiceTokenClientInterceptor(token)))
//	}

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const serviceTokenHeader = "x-service-token"

// ServiceTokenServerInterceptor returns a gRPC unary server interceptor that
// rejects calls without a matching service token in gRPC metadata.
//
// When token is empty the interceptor is a no-op — this preserves backward
// compatibility for local dev and test environments where the env var is unset.
func ServiceTokenServerInterceptor(token string) grpc.UnaryServerInterceptor {
	token = strings.TrimSpace(token)
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if token == "" {
			// No token configured — skip check (local dev / test).
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing service token")
		}
		vals := md.Get(serviceTokenHeader)
		if len(vals) == 0 || strings.TrimSpace(vals[0]) != token {
			return nil, status.Error(codes.Unauthenticated, "invalid service token")
		}
		return handler(ctx, req)
	}
}

// ServiceTokenClientInterceptor returns a gRPC unary client interceptor that
// injects the service token into every outgoing call's metadata.
//
// When token is empty the interceptor is a no-op.
func ServiceTokenClientInterceptor(token string) grpc.UnaryClientInterceptor {
	token = strings.TrimSpace(token)
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if token != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, serviceTokenHeader, token)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
