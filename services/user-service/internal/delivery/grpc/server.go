package grpc

import (
	"context"
	"errors"

	"github.com/rs/zerolog"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	apperrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/user-service/internal/domain"
	"github.com/tienlao/agregator/services/user-service/internal/usecase"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// internalErrMsg is the generic message returned to clients in place of raw
// usecase / pgx error strings. The full error is logged structurally so that
// operators can debug while callers never see column names, constraint
// internals or SQL fragments that would aid an attacker profiling the DB.
const internalErrMsg = "internal error"

type UserServer struct {
	userv1.UnimplementedUserServiceServer
	uc  *usecase.UserUseCase
	log zerolog.Logger
}

func NewUserServer(uc *usecase.UserUseCase, log zerolog.Logger) *UserServer {
	return &UserServer{uc: uc, log: log}
}

func (s *UserServer) CreateUser(ctx context.Context, req *userv1.CreateUserRequest) (*userv1.UserResponse, error) {
	if req.GetId() == "" {
		return nil, apperrors.InvalidArgument("id is required")
	}
	if req.GetEmail() == "" {
		return nil, apperrors.InvalidArgument("email is required")
	}
	if req.GetName() == "" {
		return nil, apperrors.InvalidArgument("name is required")
	}

	user := &domain.User{
		ID:    req.GetId(),
		Email: req.GetEmail(),
		Phone: req.GetPhone(),
		Name:  req.GetName(),
		Role:  req.GetRole(),
	}

	if err := s.uc.Create(ctx, user); err != nil {
		// ErrInvalidRole comes from the usecase pre-check, not the DB. Email
		// uniqueness violations (pg 23505) are mapped to AlreadyExists by
		// grpcutil.PgErrorUnaryInterceptor with a generic message.
		if errors.Is(err, domain.ErrInvalidRole) {
			return nil, apperrors.InvalidArgument("недопустимая роль пользователя")
		}
		s.log.Error().Err(err).Str("method", "CreateUser").Str("user_id", req.GetId()).Msg("create user failed")
		return nil, apperrors.Internal(internalErrMsg)
	}

	return userToProto(user), nil
}

func (s *UserServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.UserResponse, error) {
	if req.GetId() == "" {
		return nil, apperrors.InvalidArgument("id is required")
	}

	user, err := s.uc.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, apperrors.NotFound("user not found")
		}
		s.log.Error().Err(err).Str("method", "GetUser").Str("user_id", req.GetId()).Msg("get user failed")
		return nil, apperrors.Internal(internalErrMsg)
	}

	return userToProto(user), nil
}

func (s *UserServer) GetUsersBatch(ctx context.Context, req *userv1.GetUsersBatchRequest) (*userv1.GetUsersBatchResponse, error) {
	ids := req.GetIds()
	if len(ids) == 0 {
		return &userv1.GetUsersBatchResponse{Users: map[string]*userv1.UserResponse{}}, nil
	}
	if len(ids) > 200 {
		return nil, apperrors.InvalidArgument("ids: max 200 per batch request")
	}
	users, err := s.uc.GetBatch(ctx, ids)
	if err != nil {
		s.log.Error().Err(err).Str("method", "GetUsersBatch").Int("ids_count", len(ids)).Msg("get users batch failed")
		return nil, apperrors.Internal(internalErrMsg)
	}
	resp := &userv1.GetUsersBatchResponse{
		Users: make(map[string]*userv1.UserResponse, len(users)),
	}
	for id, u := range users {
		resp.Users[id] = userToProto(u)
	}
	return resp, nil
}

// GetUserByEmail is an INTERNAL-ONLY method. It is called by api-gateway for
// staff invite flows (venue_crm: invite by email) and must never be exposed
// directly as a public HTTP endpoint without:
//   - caller authentication (service-to-service token / mTLS)
//   - rate-limiting per caller identity
//
// Without these controls the endpoint allows unrestricted email enumeration:
// an attacker can probe arbitrary addresses and learn which are registered.
// The current deployment keeps user-service inside the private network so
// direct exposure is not possible today, but this boundary must be preserved
// as the topology evolves.
func (s *UserServer) GetUserByEmail(ctx context.Context, req *userv1.GetUserByEmailRequest) (*userv1.UserResponse, error) {
	if req.GetEmail() == "" {
		return nil, apperrors.InvalidArgument("email is required")
	}

	user, err := s.uc.GetByEmail(ctx, req.GetEmail())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, apperrors.NotFound("user not found")
		}
		s.log.Error().Err(err).Str("method", "GetUserByEmail").Msg("get user by email failed")
		return nil, apperrors.Internal(internalErrMsg)
	}

	return userToProto(user), nil
}

func (s *UserServer) UpdateUser(ctx context.Context, req *userv1.UpdateUserRequest) (*userv1.UserResponse, error) {
	if req.GetId() == "" {
		return nil, apperrors.InvalidArgument("id is required")
	}

	user, err := s.uc.Update(ctx, req.GetId(), req.Name, req.Phone, req.AvatarUrl, req.Bio)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, apperrors.NotFound("user not found")
		}
		s.log.Error().Err(err).Str("method", "UpdateUser").Str("user_id", req.GetId()).Msg("update user failed")
		return nil, apperrors.Internal(internalErrMsg)
	}
	if user == nil {
		return nil, apperrors.NotFound("user not found")
	}

	return userToProto(user), nil
}

func (s *UserServer) DeleteUser(ctx context.Context, req *userv1.DeleteUserRequest) (*userv1.DeleteUserResponse, error) {
	if req.GetId() == "" {
		return nil, apperrors.InvalidArgument("id is required")
	}

	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, apperrors.NotFound("user not found")
		}
		s.log.Error().Err(err).Str("method", "DeleteUser").Str("user_id", req.GetId()).Msg("delete user failed")
		return nil, apperrors.Internal(internalErrMsg)
	}

	return &userv1.DeleteUserResponse{}, nil
}

func userToProto(u *domain.User) *userv1.UserResponse {
	return &userv1.UserResponse{
		Id:        u.ID,
		Email:     u.Email,
		Phone:     u.Phone,
		Name:      u.Name,
		Role:      u.Role,
		AvatarUrl: u.AvatarURL,
		Bio:       u.Bio,
		CreatedAt: timestamppb.New(u.CreatedAt),
		UpdatedAt: timestamppb.New(u.UpdatedAt),
	}
}
