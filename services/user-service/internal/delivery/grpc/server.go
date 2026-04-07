package grpc

import (
	"context"
	"errors"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	apperrors "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/user-service/internal/domain"
	"github.com/tienlao/agregator/services/user-service/internal/usecase"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type UserServer struct {
	userv1.UnimplementedUserServiceServer
	uc *usecase.UserUseCase
}

func NewUserServer(uc *usecase.UserUseCase) *UserServer {
	return &UserServer{uc: uc}
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
		if errors.Is(err, domain.ErrEmailExists) {
			return nil, apperrors.AlreadyExists("пользователь с таким email уже существует")
		}
		if errors.Is(err, domain.ErrInvalidRole) {
			return nil, apperrors.InvalidArgument("недопустимая роль пользователя")
		}
		return nil, apperrors.Internal(err.Error())
	}

	return userToProto(user), nil
}

func (s *UserServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.UserResponse, error) {
	if req.GetId() == "" {
		return nil, apperrors.InvalidArgument("id is required")
	}

	user, err := s.uc.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, apperrors.Internal(err.Error())
	}
	if user == nil {
		return nil, apperrors.NotFound("user not found")
	}

	return userToProto(user), nil
}

func (s *UserServer) GetUserByEmail(ctx context.Context, req *userv1.GetUserByEmailRequest) (*userv1.UserResponse, error) {
	if req.GetEmail() == "" {
		return nil, apperrors.InvalidArgument("email is required")
	}

	user, err := s.uc.GetByEmail(ctx, req.GetEmail())
	if err != nil {
		return nil, apperrors.Internal(err.Error())
	}
	if user == nil {
		return nil, apperrors.NotFound("user not found")
	}

	return userToProto(user), nil
}

func (s *UserServer) UpdateUser(ctx context.Context, req *userv1.UpdateUserRequest) (*userv1.UserResponse, error) {
	if req.GetId() == "" {
		return nil, apperrors.InvalidArgument("id is required")
	}

	user, err := s.uc.Update(ctx, req.GetId(), req.Name, req.Phone, req.AvatarUrl, req.Bio)
	if err != nil {
		return nil, apperrors.Internal(err.Error())
	}
	if user == nil {
		return nil, apperrors.NotFound("user not found")
	}

	return userToProto(user), nil
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
	}
}
