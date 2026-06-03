package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	"github.com/tienlao/agregator/services/api-gateway/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVerifyEmail(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		verifyFn   func(ctx context.Context, in *authv1.VerifyEmailRequest, opts ...grpc.CallOption) (*authv1.VerifyEmailResponse, error)
		wantStatus int
		wantBody   string
	}{
		{
			name: "ok",
			body: `{"token":"tok-123"}`,
			verifyFn: func(_ context.Context, in *authv1.VerifyEmailRequest, _ ...grpc.CallOption) (*authv1.VerifyEmailResponse, error) {
				assert.Equal(t, "tok-123", in.GetToken())
				return &authv1.VerifyEmailResponse{}, nil
			},
			wantStatus: http.StatusOK,
			wantBody:   "email_verified",
		},
		{
			name:       "empty token → 400",
			body:       `{"token":"  "}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "invalid/expired token → upstream 400",
			body: `{"token":"bad"}`,
			verifyFn: func(_ context.Context, _ *authv1.VerifyEmailRequest, _ ...grpc.CallOption) (*authv1.VerifyEmailResponse, error) {
				return nil, status.Error(codes.InvalidArgument, "Ссылка подтверждения недействительна или истекла")
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewAuthHandler(&mockAuthClient{VerifyEmailFn: tt.verifyFn})
			req := httptest.NewRequest(http.MethodPost, "/auth/verify-email", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			h.VerifyEmail(rr, req)

			require.Equal(t, tt.wantStatus, rr.Code)
			if tt.wantBody != "" {
				assert.Contains(t, rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestResendVerification(t *testing.T) {
	t.Run("ok returns generic message", func(t *testing.T) {
		called := false
		h := NewAuthHandler(&mockAuthClient{
			ResendVerificationFn: func(_ context.Context, in *authv1.ResendVerificationRequest, _ ...grpc.CallOption) (*authv1.ResendVerificationResponse, error) {
				called = true
				assert.Equal(t, "a@b.com", in.GetEmail())
				return &authv1.ResendVerificationResponse{}, nil
			},
		})
		req := httptest.NewRequest(http.MethodPost, "/auth/resend-verification", strings.NewReader(`{"email":" a@b.com "}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		h.ResendVerification(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.True(t, called)
		assert.Contains(t, rr.Body.String(), resendVerificationSuccessMessageRU)
	})

	t.Run("empty email → 400", func(t *testing.T) {
		h := NewAuthHandler(&mockAuthClient{})
		req := httptest.NewRequest(http.MethodPost, "/auth/resend-verification", strings.NewReader(`{"email":""}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		h.ResendVerification(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestGetEmailVerificationStatus(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		getFn      func(ctx context.Context, in *authv1.GetEmailVerificationRequest, opts ...grpc.CallOption) (*authv1.GetEmailVerificationResponse, error)
		wantStatus int
		wantBody   string
	}{
		{
			name:   "verified true",
			userID: "u-1",
			getFn: func(_ context.Context, in *authv1.GetEmailVerificationRequest, _ ...grpc.CallOption) (*authv1.GetEmailVerificationResponse, error) {
				assert.Equal(t, "u-1", in.GetUserId())
				return &authv1.GetEmailVerificationResponse{EmailVerified: true}, nil
			},
			wantStatus: http.StatusOK,
			wantBody:   `"email_verified":true`,
		},
		{
			name:   "verified false",
			userID: "u-2",
			getFn: func(_ context.Context, _ *authv1.GetEmailVerificationRequest, _ ...grpc.CallOption) (*authv1.GetEmailVerificationResponse, error) {
				return &authv1.GetEmailVerificationResponse{EmailVerified: false}, nil
			},
			wantStatus: http.StatusOK,
			wantBody:   `"email_verified":false`,
		},
		{
			name:       "no user id → 401",
			userID:     "",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewAuthHandler(&mockAuthClient{GetEmailVerificationFn: tt.getFn})
			req := httptest.NewRequest(http.MethodGet, "/auth/email-verification", nil)
			if tt.userID != "" {
				req = req.WithContext(middleware.WithUserID(req.Context(), tt.userID))
			}
			rr := httptest.NewRecorder()

			h.GetEmailVerificationStatus(rr, req)

			require.Equal(t, tt.wantStatus, rr.Code)
			if tt.wantBody != "" {
				assert.Contains(t, strings.ReplaceAll(rr.Body.String(), " ", ""), tt.wantBody)
			}
		})
	}
}
