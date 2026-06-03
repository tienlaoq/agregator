package grpc

import (
	"context"

	"github.com/rs/zerolog"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/tienlao/agregator/services/auth-service/internal/usecase"
)

type Server struct {
	authv1.UnimplementedAuthServiceServer
	uc  *usecase.AuthUseCase
	log zerolog.Logger
}

func NewServer(uc *usecase.AuthUseCase, log zerolog.Logger) *Server {
	return &Server{uc: uc, log: log}
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
		Provider:      req.Provider,
		ProviderID:    req.ProviderId,
		Email:         req.Email,
		Name:          req.Name,
		AvatarURL:     req.AvatarUrl,
		EmailVerified: req.EmailVerified,
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
		// ValidateToken is a hot path called on every authenticated request.
		// We intentionally return Valid:false rather than a gRPC error so the
		// gateway can distinguish "bad token" from "auth-service unavailable".
		// Debug level keeps noise low in production; raise to Info if you need
		// to audit rejection rates (pair with sampling to avoid log flooding).
		s.log.Debug().Err(err).Msg("token validation failed")
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

func (s *Server) DeleteAccount(ctx context.Context, req *authv1.DeleteAccountRequest) (*authv1.DeleteAccountResponse, error) {
	if err := s.uc.DeleteAccount(ctx, req.GetUserId()); err != nil {
		return nil, err
	}
	return &authv1.DeleteAccountResponse{}, nil
}

func (s *Server) RequestPasswordReset(ctx context.Context, req *authv1.RequestPasswordResetRequest) (*authv1.RequestPasswordResetResponse, error) {
	if err := s.uc.RequestPasswordReset(ctx, req.GetEmail()); err != nil {
		return nil, err
	}
	return &authv1.RequestPasswordResetResponse{}, nil
}

func (s *Server) CompletePasswordReset(ctx context.Context, req *authv1.CompletePasswordResetRequest) (*authv1.CompletePasswordResetResponse, error) {
	if err := s.uc.CompletePasswordReset(ctx, req.GetToken(), req.GetNewPassword()); err != nil {
		return nil, err
	}
	return &authv1.CompletePasswordResetResponse{}, nil
}

func (s *Server) VerifyEmail(ctx context.Context, req *authv1.VerifyEmailRequest) (*authv1.VerifyEmailResponse, error) {
	if err := s.uc.VerifyEmail(ctx, req.GetToken()); err != nil {
		return nil, err
	}
	return &authv1.VerifyEmailResponse{}, nil
}

func (s *Server) ResendVerification(ctx context.Context, req *authv1.ResendVerificationRequest) (*authv1.ResendVerificationResponse, error) {
	if err := s.uc.ResendVerification(ctx, req.GetEmail()); err != nil {
		return nil, err
	}
	return &authv1.ResendVerificationResponse{}, nil
}

func (s *Server) GetEmailVerification(ctx context.Context, req *authv1.GetEmailVerificationRequest) (*authv1.GetEmailVerificationResponse, error) {
	verified, err := s.uc.GetEmailVerified(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &authv1.GetEmailVerificationResponse{EmailVerified: verified}, nil
}
