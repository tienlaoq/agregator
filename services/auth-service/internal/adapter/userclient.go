// Package adapter contains thin wrappers that translate between the domain
// interfaces defined in internal/domain and external transports (gRPC, HTTP).
// Nothing in usecase or domain imports this package — dependency flow is:
//
//	cmd → adapter → gen/go/user/v1
//	cmd → usecase → domain
//	adapter implements domain.UserClient
package adapter

import (
	"context"
	"fmt"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

// UserClientAdapter wraps a gRPC UserServiceClient and implements domain.UserClient.
type UserClientAdapter struct {
	grpc userv1.UserServiceClient
}

// NewUserClientAdapter returns a domain.UserClient backed by the supplied gRPC stub.
func NewUserClientAdapter(grpc userv1.UserServiceClient) *UserClientAdapter {
	return &UserClientAdapter{grpc: grpc}
}

// CreateUser forwards the request to user-service and maps the response to
// domain.UserRecord. Proto-specific types never leave this package.
func (a *UserClientAdapter) CreateUser(ctx context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
	resp, err := a.grpc.CreateUser(ctx, &userv1.CreateUserRequest{
		Id:    in.ID,
		Email: in.Email,
		Phone: in.Phone,
		Name:  in.Name,
		Role:  in.Role,
	})
	if err != nil {
		return nil, fmt.Errorf("user-service CreateUser: %w", err)
	}
	return &domain.UserRecord{
		ID:    resp.Id,
		Email: resp.Email,
		Role:  resp.Role,
	}, nil
}

// GetUser fetches a single user by ID from user-service.
func (a *UserClientAdapter) GetUser(ctx context.Context, userID string) (*domain.UserRecord, error) {
	resp, err := a.grpc.GetUser(ctx, &userv1.GetUserRequest{Id: userID})
	if err != nil {
		return nil, fmt.Errorf("user-service GetUser: %w", err)
	}
	return &domain.UserRecord{
		ID:    resp.Id,
		Email: resp.Email,
		Role:  resp.Role,
	}, nil
}
