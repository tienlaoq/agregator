package errors

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NotFound(msg string) error {
	return status.Error(codes.NotFound, msg)
}

func AlreadyExists(msg string) error {
	return status.Error(codes.AlreadyExists, msg)
}

func InvalidArgument(msg string) error {
	return status.Error(codes.InvalidArgument, msg)
}

func Internal(msg string) error {
	return status.Error(codes.Internal, msg)
}

func Unauthenticated(msg string) error {
	return status.Error(codes.Unauthenticated, msg)
}

func PermissionDenied(msg string) error {
	return status.Error(codes.PermissionDenied, msg)
}

func Unavailable(msg string) error {
	return status.Error(codes.Unavailable, msg)
}

func Aborted(msg string) error {
	return status.Error(codes.Aborted, msg)
}

func FailedPrecondition(msg string) error {
	return status.Error(codes.FailedPrecondition, msg)
}

// InvalidArgumentField returns an InvalidArgument status with a structured
// google.rpc.BadRequest detail that names the offending field. Callers should
// prefer this over InvalidArgument(field + " is invalid") because:
//
//   - The field name stays in the structured detail (machine-readable by the
//     client), not in the free-text message (which may be displayed verbatim
//     to end-users by API gateways or mobile clients).
//   - The human-readable message is generic ("malformed identifier") so no
//     internal field naming convention is leaked into user-facing strings.
//
// If the BadRequest detail cannot be attached (should never happen in practice),
// the function falls back to a plain InvalidArgument with the field name in the
// message so the error remains informative for developers.
func InvalidArgumentField(field, description string) error {
	st := status.New(codes.InvalidArgument, "malformed identifier")
	br := &errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{
			{Field: field, Description: description},
		},
	}
	detailed, err := st.WithDetails(br)
	if err != nil {
		// Fallback: return a plain status. This path is unreachable in practice
		// because WithDetails only fails if the detail is not registered with
		// the proto registry, and errdetails types always are.
		return status.Errorf(codes.InvalidArgument, "invalid %s: %s", field, description)
	}
	return detailed.Err()
}
