package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userv1 "github.com/tienlao/agregator/gen/go/user/v1"
	"github.com/tienlao/agregator/pkg/auth"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

const testJWTSecret = "test-jwt-secret-key"

func testAuthUC(creds *mockCredRepo, tokens *mockTokenRepo, users *mockUserClient) *AuthUseCase {
	return NewAuthUseCase(creds, tokens, users, testJWTSecret, time.Hour, 24*time.Hour, nil, "", zerolog.Nop())
}

func TestRegister_Success(t *testing.T) {
	ctx := context.Background()
	const wantUserID = "new-user-id"

	var credCreateCalled bool
	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, cred *domain.Credential) error {
			credCreateCalled = true
			require.Equal(t, wantUserID, cred.UserID)
			require.Equal(t, "reg@example.com", cred.Email)
			require.NotEmpty(t, cred.PasswordHash)
			return nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, rt *domain.RefreshToken) error {
			require.Equal(t, wantUserID, rt.UserID)
			require.NotEmpty(t, rt.TokenHash)
			return nil
		},
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, in *userv1.CreateUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
			require.Equal(t, "reg@example.com", in.Email)
			require.Equal(t, "user", in.Role)
			return &userv1.UserResponse{Id: wantUserID, Email: in.Email, Role: in.Role}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	res, err := uc.Register(ctx, RegisterInput{
		Email:    "reg@example.com",
		Phone:    "+1000",
		Password: "register-pass",
		Name:     "Reg User",
		Role:     "user",
	})
	require.NoError(t, err)
	require.True(t, credCreateCalled)
	require.Equal(t, wantUserID, res.UserID)
	require.NotEmpty(t, res.Tokens.AccessToken)
	require.NotEmpty(t, res.Tokens.RefreshToken)
}

func TestLogin_Success(t *testing.T) {
	ctx := context.Background()
	hash, err := hashPassword("testpass123")
	require.NoError(t, err)

	const userID = "login-user-1"
	const email = "login@example.com"

	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, e string) (*domain.Credential, error) {
			require.Equal(t, email, e)
			return &domain.Credential{UserID: userID, Email: email, PasswordHash: hash}, nil
		},
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, rt *domain.RefreshToken) error {
			require.Equal(t, userID, rt.UserID)
			return nil
		},
	}
	users := &mockUserClient{
		GetUserFunc: func(_ context.Context, in *userv1.GetUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
			require.Equal(t, userID, in.Id)
			return &userv1.UserResponse{Id: userID, Email: email, Role: "owner"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	res, err := uc.Login(ctx, email, "testpass123")
	require.NoError(t, err)
	require.Equal(t, userID, res.UserID)
	require.NotEmpty(t, res.Tokens.AccessToken)
	require.NotEmpty(t, res.Tokens.RefreshToken)
}

func TestLogin_WrongPassword(t *testing.T) {
	ctx := context.Background()
	hash, err := hashPassword("testpass123")
	require.NoError(t, err)

	const email = "wrong@example.com"
	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, e string) (*domain.Credential, error) {
			return &domain.Credential{UserID: "u", Email: e, PasswordHash: hash}, nil
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, &mockUserClient{})
	_, err = uc.Login(ctx, email, "not-the-password")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestLogin_UserNotFound(t *testing.T) {
	ctx := context.Background()
	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return nil, errors.New("no rows")
		},
	}

	uc := testAuthUC(creds, &mockTokenRepo{}, &mockUserClient{})
	_, err := uc.Login(ctx, "missing@example.com", "any")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestValidateToken_Valid(t *testing.T) {
	ctx := context.Background()
	const wantUserID = "valid-token-user"

	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, _ *domain.Credential) error { return nil },
	}
	tokens := &mockTokenRepo{
		CreateFunc: func(_ context.Context, _ *domain.RefreshToken) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, _ *userv1.CreateUserRequest, _ ...grpc.CallOption) (*userv1.UserResponse, error) {
			return &userv1.UserResponse{Id: wantUserID, Email: "vt@example.com", Role: "user"}, nil
		},
	}

	uc := testAuthUC(creds, tokens, users)
	reg, err := uc.Register(ctx, RegisterInput{
		Email:    "vt@example.com",
		Password: "p",
		Name:     "V",
		Role:     "user",
	})
	require.NoError(t, err)

	claims, err := uc.ValidateToken(ctx, reg.Tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, wantUserID, claims.UserID)
	require.Equal(t, "vt@example.com", claims.Email)
	require.Equal(t, "user", claims.Role)
}

func TestValidateToken_Expired(t *testing.T) {
	ctx := context.Background()
	expired, err := auth.GenerateAccessToken(testJWTSecret, "u1", "e@x.com", "user", -time.Hour)
	require.NoError(t, err)

	uc := testAuthUC(&mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{})
	_, err = uc.ValidateToken(ctx, expired)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}
