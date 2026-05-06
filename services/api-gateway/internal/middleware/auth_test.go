package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"google.golang.org/grpc"
)

// mockAuthClient implements authv1.AuthServiceClient for tests. Methods other than
// ValidateToken return nil, nil unless customized later.
type mockAuthClient struct {
	validateToken func(ctx context.Context, in *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error)
}

func (m *mockAuthClient) Register(ctx context.Context, in *authv1.RegisterRequest, opts ...grpc.CallOption) (*authv1.RegisterResponse, error) {
	return nil, nil
}

func (m *mockAuthClient) Login(ctx context.Context, in *authv1.LoginRequest, opts ...grpc.CallOption) (*authv1.LoginResponse, error) {
	return nil, nil
}

func (m *mockAuthClient) OAuthLogin(ctx context.Context, in *authv1.OAuthLoginRequest, opts ...grpc.CallOption) (*authv1.OAuthLoginResponse, error) {
	return nil, nil
}

func (m *mockAuthClient) RefreshToken(ctx context.Context, in *authv1.RefreshTokenRequest, opts ...grpc.CallOption) (*authv1.RefreshTokenResponse, error) {
	return nil, nil
}

func (m *mockAuthClient) ValidateToken(ctx context.Context, in *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
	if m.validateToken != nil {
		return m.validateToken(ctx, in, opts...)
	}
	return nil, nil
}

func (m *mockAuthClient) Logout(ctx context.Context, in *authv1.LogoutRequest, opts ...grpc.CallOption) (*authv1.LogoutResponse, error) {
	return nil, nil
}

func (m *mockAuthClient) RequestPasswordReset(ctx context.Context, in *authv1.RequestPasswordResetRequest, opts ...grpc.CallOption) (*authv1.RequestPasswordResetResponse, error) {
	return &authv1.RequestPasswordResetResponse{}, nil
}

func (m *mockAuthClient) CompletePasswordReset(ctx context.Context, in *authv1.CompletePasswordResetRequest, opts ...grpc.CallOption) (*authv1.CompletePasswordResetResponse, error) {
	return &authv1.CompletePasswordResetResponse{}, nil
}

func TestAuth_MissingHeader(t *testing.T) {
	client := &mockAuthClient{}
	h := Auth(client, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing authorization header")
}

func TestAuth_InvalidFormat(t *testing.T) {
	client := &mockAuthClient{}
	h := Auth(client, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "invalid")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid authorization header")
}

func TestAuth_InvalidToken(t *testing.T) {
	client := &mockAuthClient{
		validateToken: func(ctx context.Context, in *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
			return &authv1.ValidateTokenResponse{Valid: false}, nil
		},
	}
	h := Auth(client, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid or expired token")
}

func TestAuth_ValidToken(t *testing.T) {
	var gotToken string
	client := &mockAuthClient{
		validateToken: func(ctx context.Context, in *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
			gotToken = in.GetAccessToken()
			return &authv1.ValidateTokenResponse{
				Valid:  true,
				UserId: "user-1",
				Role:   "customer",
				Email:  "a@b.c",
			}, nil
		},
	}

	var uid, role, email string
	h := Auth(client, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid = UserIDFromCtx(r.Context())
		role = RoleFromCtx(r.Context())
		email = EmailFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer good-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "good-token", gotToken)
	assert.Equal(t, "user-1", uid)
	assert.Equal(t, "customer", role)
	assert.Equal(t, "a@b.c", email)
}

func TestAuth_ValidTokenFromQueryParam(t *testing.T) {
	var gotToken string
	client := &mockAuthClient{
		validateToken: func(ctx context.Context, in *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
			gotToken = in.GetAccessToken()
			return &authv1.ValidateTokenResponse{
				Valid:  true,
				UserId: "user-1",
				Role:   "customer",
				Email:  "a@b.c",
			}, nil
		},
	}

	h := Auth(client, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/chat/ws?access_token=good-token", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "good-token", gotToken)
}

func TestAuth_ServiceError(t *testing.T) {
	client := &mockAuthClient{
		validateToken: func(ctx context.Context, in *authv1.ValidateTokenRequest, opts ...grpc.CallOption) (*authv1.ValidateTokenResponse, error) {
			return nil, errors.New("rpc failed")
		},
	}
	h := Auth(client, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer t")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid or expired token")
}

func TestRequireRole_Allowed(t *testing.T) {
	called := false
	h := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	ctx := context.WithValue(context.Background(), CtxRole, "admin")
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireRole_Forbidden(t *testing.T) {
	h := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	}))

	ctx := context.WithValue(context.Background(), CtxRole, "guest")
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "forbidden")
}

func TestRequireRole_MultipleRoles(t *testing.T) {
	called := false
	h := RequireRole("admin", "partner", "owner")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	ctx := context.WithValue(context.Background(), CtxRole, "partner")
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestUserIDFromCtx(t *testing.T) {
	ctx := context.WithValue(context.Background(), CtxUserID, "u-42")
	assert.Equal(t, "u-42", UserIDFromCtx(ctx))
}

func TestUserIDFromCtx_Empty(t *testing.T) {
	assert.Equal(t, "", UserIDFromCtx(context.Background()))
}
