package apicatalog

import (
	"google.golang.org/grpc/codes"
)

// FromGRPC returns a catalog entry for a gRPC status code (default messages; use WithMessage for detail).
func FromGRPC(c codes.Code) (Entry, bool) {
	switch c {
	case codes.InvalidArgument:
		return GatewayUpstreamInvalidArgument, true
	case codes.NotFound:
		return GatewayUpstreamNotFound, true
	case codes.AlreadyExists:
		return GatewayUpstreamAlreadyExists, true
	case codes.Unauthenticated:
		return GatewayUpstreamUnauthenticated, true
	case codes.PermissionDenied:
		return GatewayUpstreamPermissionDenied, true
	case codes.Unavailable:
		return GatewayUpstreamUnavailable, true
	case codes.FailedPrecondition:
		return GatewayUpstreamFailedPrecondition, true
	case codes.Internal:
		return GatewayUpstreamInternal, true
	case codes.Unknown:
		return GatewayUpstreamUnknown, true
	default:
		return Entry{}, false
	}
}
