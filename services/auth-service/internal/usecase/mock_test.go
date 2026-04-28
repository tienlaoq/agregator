package usecase

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

type mockCredRepo struct {
	CreateFunc             func(ctx context.Context, cred *domain.Credential) error
	GetByEmailFunc         func(ctx context.Context, email string) (*domain.Credential, error)
	GetByUserIDFunc        func(ctx context.Context, userID string) (*domain.Credential, error)
	GetByProviderFunc      func(ctx context.Context, provider, providerID string) (*domain.Credential, error)
	CreateOAuthFunc        func(ctx context.Context, cred *domain.Credential) error
	UpdatePasswordHashFunc func(ctx context.Context, userID, passwordHash string) error
}

func (m *mockCredRepo) Create(ctx context.Context, cred *domain.Credential) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, cred)
	}
	return nil
}

func (m *mockCredRepo) GetByEmail(ctx context.Context, email string) (*domain.Credential, error) {
	if m.GetByEmailFunc != nil {
		return m.GetByEmailFunc(ctx, email)
	}
	return nil, nil
}

func (m *mockCredRepo) GetByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	if m.GetByUserIDFunc != nil {
		return m.GetByUserIDFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockCredRepo) GetByProvider(ctx context.Context, provider, providerID string) (*domain.Credential, error) {
	if m.GetByProviderFunc != nil {
		return m.GetByProviderFunc(ctx, provider, providerID)
	}
	return nil, nil
}

func (m *mockCredRepo) CreateOAuth(ctx context.Context, cred *domain.Credential) error {
	if m.CreateOAuthFunc != nil {
		return m.CreateOAuthFunc(ctx, cred)
	}
	return nil
}

func (m *mockCredRepo) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	if m.UpdatePasswordHashFunc != nil {
		return m.UpdatePasswordHashFunc(ctx, userID, passwordHash)
	}
	return nil
}

type mockTokenRepo struct {
	CreateFunc         func(ctx context.Context, token *domain.RefreshToken) error
	GetByHashFunc      func(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
	DeleteByUserIDFunc func(ctx context.Context, userID string) error
	DeleteByHashFunc   func(ctx context.Context, tokenHash string) error
}

func (m *mockTokenRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, token)
	}
	return nil
}

func (m *mockTokenRepo) GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	if m.GetByHashFunc != nil {
		return m.GetByHashFunc(ctx, tokenHash)
	}
	return nil, nil
}

func (m *mockTokenRepo) DeleteByUserID(ctx context.Context, userID string) error {
	if m.DeleteByUserIDFunc != nil {
		return m.DeleteByUserIDFunc(ctx, userID)
	}
	return nil
}

func (m *mockTokenRepo) DeleteByHash(ctx context.Context, tokenHash string) error {
	if m.DeleteByHashFunc != nil {
		return m.DeleteByHashFunc(ctx, tokenHash)
	}
	return nil
}

type mockUserClient struct {
	CreateUserFunc     func(ctx context.Context, in *userv1.CreateUserRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error)
	GetUserFunc        func(ctx context.Context, in *userv1.GetUserRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error)
	UpdateUserFunc     func(ctx context.Context, in *userv1.UpdateUserRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error)
	GetUserByEmailFunc func(ctx context.Context, in *userv1.GetUserByEmailRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error)
}

func (m *mockUserClient) CreateUser(ctx context.Context, in *userv1.CreateUserRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockUserClient) GetUser(ctx context.Context, in *userv1.GetUserRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error) {
	if m.GetUserFunc != nil {
		return m.GetUserFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockUserClient) UpdateUser(ctx context.Context, in *userv1.UpdateUserRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error) {
	if m.UpdateUserFunc != nil {
		return m.UpdateUserFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockUserClient) GetUserByEmail(ctx context.Context, in *userv1.GetUserByEmailRequest, opts ...grpc.CallOption) (*userv1.UserResponse, error) {
	if m.GetUserByEmailFunc != nil {
		return m.GetUserByEmailFunc(ctx, in, opts...)
	}
	return nil, nil
}

type mockPasswordResetRepo struct {
	InvalidateUnusedByUserIDFunc func(ctx context.Context, userID string) error
	CreateFunc                   func(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ConsumeByTokenHashFunc       func(ctx context.Context, tokenHash string) (string, error)
}

func (m *mockPasswordResetRepo) InvalidateUnusedByUserID(ctx context.Context, userID string) error {
	if m.InvalidateUnusedByUserIDFunc != nil {
		return m.InvalidateUnusedByUserIDFunc(ctx, userID)
	}
	return nil
}

func (m *mockPasswordResetRepo) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, userID, tokenHash, expiresAt)
	}
	return nil
}

func (m *mockPasswordResetRepo) ConsumeByTokenHash(ctx context.Context, tokenHash string) (string, error) {
	if m.ConsumeByTokenHashFunc != nil {
		return m.ConsumeByTokenHashFunc(ctx, tokenHash)
	}
	return "", errors.New("consume not stubbed")
}

type mockPasswordMail struct {
	enabled    bool
	sendErr    error
	lastTo     string
	lastToken  string
	sendCalled int
}

func (m *mockPasswordMail) Enabled() bool { return m != nil && m.enabled }

func (m *mockPasswordMail) SendPasswordReset(ctx context.Context, toEmail, rawToken string) error {
	if m == nil {
		return nil
	}
	m.sendCalled++
	m.lastTo = toEmail
	m.lastToken = rawToken
	return m.sendErr
}

type noopPasswordResetRepo struct{}

func (noopPasswordResetRepo) InvalidateUnusedByUserID(context.Context, string) error { return nil }
func (noopPasswordResetRepo) Create(context.Context, string, string, time.Time) error {
	return nil
}
func (noopPasswordResetRepo) ConsumeByTokenHash(context.Context, string) (string, error) {
	return "", errors.New("noop reset repo")
}

type noopPasswordMail struct{}

func (noopPasswordMail) Enabled() bool { return false }
func (noopPasswordMail) SendPasswordReset(context.Context, string, string) error {
	return nil
}
