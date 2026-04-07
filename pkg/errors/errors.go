package errors

import (
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
