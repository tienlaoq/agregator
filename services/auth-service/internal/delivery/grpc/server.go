package grpc

import (
	"context"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/tienlao/agregator/services/auth-service/internal/usecase"
)

type Server struct {
	authv1.UnimplementedAuthServiceServer
	uc *usecase.AuthUseCase
}

func NewServer(uc *usecase.AuthUseCase) *Server {
	return &Server{uc: uc}
}

func (s *Server) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	result, err := s.uc.Register(ctx, usecase.RegisterInput{
		Email:    req.Email,
		Phone:    req.Phone,
		Password: req.Password,
		Name:     req.Name,
		Role:     req.Role,
	})
	if err != nil {
		return nil, err
	}

	return &authv1.RegisterResponse{
		UserId:       result.UserID,
		AccessToken:  result.Tokens.AccessToken,
		RefreshToken: result.Tokens.RefreshToken,
	}, nil
}

func (s *Server) OAuthLogin(ctx context.Context, req *authv1.OAuthLoginRequest) (*authv1.OAuthLoginResponse, error) {
	result, err := s.uc.OAuthLogin(ctx, usecase.OAuthInput{
		Provider:   req.Provider,
		ProviderID: req.ProviderId,
		Email:      req.Email,
		Name:       req.Name,
		AvatarURL:  req.AvatarUrl,
	})
	if err != nil {
		return nil, err
	}

	return &authv1.OAuthLoginResponse{
		UserId:       result.UserID,
		AccessToken:  result.Tokens.AccessToken,
		RefreshToken: result.Tokens.RefreshToken,
		IsNewUser:    result.IsNewUser,
	}, nil
}

func (s *Server) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	result, err := s.uc.Login(ctx, req.Email, req.Password)
	if err != nil {
		return nil, err
	}

	return &authv1.LoginResponse{
		UserId:       result.UserID,
		AccessToken:  result.Tokens.AccessToken,
		RefreshToken: result.Tokens.RefreshToken,
	}, nil
}

func (s *Server) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	tokens, err := s.uc.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		return nil, err
	}

	return &authv1.RefreshTokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
	}, nil
}

func (s *Server) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	claims, err := s.uc.ValidateToken(ctx, req.AccessToken)
	if err != nil {
		return &authv1.ValidateTokenResponse{Valid: false}, nil
	}

	return &authv1.ValidateTokenResponse{
		Valid:  true,
		UserId: claims.UserID,
		Role:   claims.Role,
		Email:  claims.Email,
	}, nil
}

func (s *Server) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	if err := s.uc.Logout(ctx, req.RefreshToken); err != nil {
		return nil, err
	}
	return &authv1.LogoutResponse{}, nil
}
