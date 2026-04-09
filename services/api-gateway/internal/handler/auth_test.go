package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockAuthClient implements authv1.AuthServiceClient with nil,nil defaults and optional hooks.
type mockAuthClient struct {
	RegisterFn      func(ctx context.Context, in *authv1.RegisterRequest, opts ...grpc.CallOption) (*authv1.RegisterResponse, error)
	LoginFn         func(ctx context.Context, in *authv1.LoginRequest, opts ...grpc.CallOption) (*authv1.LoginResponse, error)
	OAuthLoginFn    func(ctx context.Context, in *authv1.OAuthLoginRequest, opts ...grpc.CallOption) (*authv1.OAuthLoginResponse, error)
	RefreshTokenFn  func(ctx context.Context, in *authv1.RefreshTokenRequest, opts ...grpc.CallOption) (*authv1.RefreshTokenResponse, error)
	ValidateTokenFn func(ctx context.Context, in *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error)
	LogoutFn        func(ctx context.Context, in *authv1.LogoutRequest, opts ...grpc.CallOption) (*authv1.LogoutResponse, error)
}

func (m *mockAuthClient) Register(ctx context.Context, in *authv1.RegisterRequest, opts ...grpc.CallOption) (*authv1.RegisterResponse, error) {
	if m.RegisterFn != nil {
		return m.RegisterFn(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockAuthClient) Login(ctx context.Context, in *authv1.LoginRequest, opts ...grpc.CallOption) (*authv1.LoginResponse, error) {
	if m.LoginFn != nil {
		return m.LoginFn(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockAuthClient) OAuthLogin(ctx context.Context, in *authv1.OAuthLoginRequest, opts ...grpc.CallOption) (*authv1.OAuthLoginResponse, error) {
	if m.OAuthLoginFn != nil {
		return m.OAuthLoginFn(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockAuthClient) RefreshToken(ctx context.Context, in *authv1.RefreshTokenRequest, opts ...grpc.CallOption) (*authv1.RefreshTokenResponse, error) {
	if m.RefreshTokenFn != nil {
		return m.RefreshTokenFn(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockAuthClient) ValidateToken(ctx context.Context, in *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
	if m.ValidateTokenFn != nil {
		return m.ValidateTokenFn(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockAuthClient) Logout(ctx context.Context, in *authv1.LogoutRequest, opts ...grpc.CallOption) (*authv1.LogoutResponse, error) {
	if m.LogoutFn != nil {
		return m.LogoutFn(ctx, in, opts...)
	}
	return nil, nil
}

func TestRegister_Success(t *testing.T) {
	mock := &mockAuthClient{
		RegisterFn: func(ctx context.Context, in *authv1.RegisterRequest, _ ...grpc.CallOption) (*authv1.RegisterResponse, error) {
			assert.Equal(t, "a@b.com", in.GetEmail())
			assert.Equal(t, "+100", in.GetPhone())
			assert.Equal(t, "secret", in.GetPassword())
			assert.Equal(t, "Ann", in.GetName())
			assert.Equal(t, "user", in.GetRole())
			return &authv1.RegisterResponse{
				UserId:       "uid-1",
				AccessToken:  "at",
				RefreshToken: "rt",
			}, nil
		},
	}
	h := NewAuthHandler(mock)
	body := `{"email":"a@b.com","phone":"+100","password":"secret","name":"Ann","role":"user"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, "uid-1", out["user_id"])
	assert.Equal(t, "at", out["access_token"])
	assert.Equal(t, "rt", out["refresh_token"])
}

func TestRegister_InvalidBody(t *testing.T) {
	h := NewAuthHandler(&mockAuthClient{})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var out map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, "invalid request body", out["error"])
}

func TestRegister_GRPCError(t *testing.T) {
	mock := &mockAuthClient{
		RegisterFn: func(ctx context.Context, in *authv1.RegisterRequest, _ ...grpc.CallOption) (*authv1.RegisterResponse, error) {
			return nil, status.Error(codes.AlreadyExists, "email taken")
		},
	}
	h := NewAuthHandler(mock)
	body := `{"email":"x@y.com","password":"p","name":"n","role":"user"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	var out map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, "email taken", out["error"])
}

func TestLogin_Success(t *testing.T) {
	mock := &mockAuthClient{
		LoginFn: func(ctx context.Context, in *authv1.LoginRequest, _ ...grpc.CallOption) (*authv1.LoginResponse, error) {
			assert.Equal(t, "u@u.com", in.GetEmail())
			assert.Equal(t, "pw", in.GetPassword())
			return &authv1.LoginResponse{
				UserId:       "u1",
				AccessToken:  "acc",
				RefreshToken: "ref",
			}, nil
		},
	}
	h := NewAuthHandler(mock)
	body := `{"email":"u@u.com","password":"pw"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, "u1", out["user_id"])
	assert.Equal(t, "acc", out["access_token"])
	assert.Equal(t, "ref", out["refresh_token"])
}

func TestLogin_WrongCredentials(t *testing.T) {
	mock := &mockAuthClient{
		LoginFn: func(ctx context.Context, in *authv1.LoginRequest, _ ...grpc.CallOption) (*authv1.LoginResponse, error) {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		},
	}
	h := NewAuthHandler(mock)
	body := `{"email":"u@u.com","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var out map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, "invalid credentials", out["error"])
}

func TestRefreshToken_Success(t *testing.T) {
	mock := &mockAuthClient{
		RefreshTokenFn: func(ctx context.Context, in *authv1.RefreshTokenRequest, _ ...grpc.CallOption) (*authv1.RefreshTokenResponse, error) {
			assert.Equal(t, "old-rt", in.GetRefreshToken())
			return &authv1.RefreshTokenResponse{
				AccessToken:  "new-at",
				RefreshToken: "new-rt",
			}, nil
		},
	}
	h := NewAuthHandler(mock)
	body := `{"refresh_token":"old-rt"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.RefreshToken(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, "new-at", out["access_token"])
	assert.Equal(t, "new-rt", out["refresh_token"])
}

func TestRefreshToken_Invalid(t *testing.T) {
	mock := &mockAuthClient{
		RefreshTokenFn: func(ctx context.Context, in *authv1.RefreshTokenRequest, _ ...grpc.CallOption) (*authv1.RefreshTokenResponse, error) {
			return nil, status.Error(codes.Unauthenticated, "bad refresh")
		},
	}
	h := NewAuthHandler(mock)
	body := `{"refresh_token":"expired"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.RefreshToken(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	var out map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, "bad refresh", out["error"])
}

func TestLogout_Success(t *testing.T) {
	mock := &mockAuthClient{
		LogoutFn: func(ctx context.Context, in *authv1.LogoutRequest, _ ...grpc.CallOption) (*authv1.LogoutResponse, error) {
			assert.Equal(t, "rt-to-revoke", in.GetRefreshToken())
			return &authv1.LogoutResponse{}, nil
		},
	}
	h := NewAuthHandler(mock)
	body := `{"refresh_token":"rt-to-revoke"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Logout(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, "logged_out", out["status"])
}
