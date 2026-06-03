package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

// testAuthUCVerify builds a usecase wired with the supplied verification repo
// and mailer so the email-verification paths can be exercised in isolation.
func testAuthUCVerify(
	creds *mockCredRepo,
	tokens *mockTokenRepo,
	users *mockUserClient,
	verifyRepo domain.EmailVerificationRepository,
	verifyMail EmailVerificationMailer,
) *AuthUseCase {
	if verifyRepo == nil {
		verifyRepo = &mockEmailVerificationRepo{}
	}
	if verifyMail == nil {
		verifyMail = &mockVerifyMail{}
	}
	return NewAuthUseCase(
		creds, tokens,
		noopPasswordResetRepo{}, noopPasswordMail{},
		time.Hour,
		verifyRepo, verifyMail,
		24*time.Hour,
		users, testPrivKey, time.Hour, 24*time.Hour,
		nil,
		zerolog.Nop(),
	)
}

func TestVerifyEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("success marks credential verified", func(t *testing.T) {
		var setUserID string
		var setValue bool
		creds := &mockCredRepo{
			SetEmailVerifiedFunc: func(_ context.Context, userID string, verified bool) error {
				setUserID = userID
				setValue = verified
				return nil
			},
		}
		verifyRepo := &mockEmailVerificationRepo{
			ConsumeByTokenHashFunc: func(_ context.Context, _ string) (string, error) {
				return "user-42", nil
			},
		}
		uc := testAuthUCVerify(creds, &mockTokenRepo{}, &mockUserClient{}, verifyRepo, nil)

		err := uc.VerifyEmail(ctx, "raw-token")
		require.NoError(t, err)
		assert.Equal(t, "user-42", setUserID)
		assert.True(t, setValue)
	})

	t.Run("empty token rejected", func(t *testing.T) {
		uc := testAuthUCVerify(&mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{}, nil, nil)
		err := uc.VerifyEmail(ctx, "   ")
		require.Error(t, err)
		st, _ := status.FromError(err)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("invalid or expired token maps to InvalidArgument", func(t *testing.T) {
		verifyRepo := &mockEmailVerificationRepo{
			ConsumeByTokenHashFunc: func(_ context.Context, _ string) (string, error) {
				return "", pkgerr.NotFound("verification token invalid or expired")
			},
		}
		uc := testAuthUCVerify(&mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{}, verifyRepo, nil)
		err := uc.VerifyEmail(ctx, "stale")
		require.Error(t, err)
		st, _ := status.FromError(err)
		assert.Equal(t, codes.InvalidArgument, st.Code())
	})
}

func TestResendVerification(t *testing.T) {
	ctx := context.Background()

	t.Run("unverified account triggers send", func(t *testing.T) {
		creds := &mockCredRepo{
			GetByEmailFunc: func(_ context.Context, email string) (*domain.Credential, error) {
				assert.Equal(t, "user@example.com", email)
				return &domain.Credential{UserID: "u1", Email: "user@example.com", EmailVerified: false}, nil
			},
		}
		mail := &mockVerifyMail{enabled: true}
		uc := testAuthUCVerify(creds, &mockTokenRepo{}, &mockUserClient{}, &mockEmailVerificationRepo{}, mail)

		err := uc.ResendVerification(ctx, "  User@Example.com  ")
		require.NoError(t, err)
		assert.Equal(t, 1, mail.sendCalled)
		assert.Equal(t, "user@example.com", mail.lastTo)
	})

	t.Run("already verified stays silent and does not send", func(t *testing.T) {
		creds := &mockCredRepo{
			GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
				return &domain.Credential{UserID: "u1", Email: "user@example.com", EmailVerified: true}, nil
			},
		}
		mail := &mockVerifyMail{enabled: true}
		uc := testAuthUCVerify(creds, &mockTokenRepo{}, &mockUserClient{}, &mockEmailVerificationRepo{}, mail)

		err := uc.ResendVerification(ctx, "user@example.com")
		require.NoError(t, err)
		assert.Equal(t, 0, mail.sendCalled)
	})

	t.Run("unknown email stays silent and does not send", func(t *testing.T) {
		creds := &mockCredRepo{
			GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
				return nil, pkgerr.NotFound("credential not found")
			},
		}
		mail := &mockVerifyMail{enabled: true}
		uc := testAuthUCVerify(creds, &mockTokenRepo{}, &mockUserClient{}, &mockEmailVerificationRepo{}, mail)

		err := uc.ResendVerification(ctx, "ghost@example.com")
		require.NoError(t, err)
		assert.Equal(t, 0, mail.sendCalled)
	})

	t.Run("empty email is a no-op", func(t *testing.T) {
		mail := &mockVerifyMail{enabled: true}
		uc := testAuthUCVerify(&mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{}, &mockEmailVerificationRepo{}, mail)
		require.NoError(t, uc.ResendVerification(ctx, "   "))
		assert.Equal(t, 0, mail.sendCalled)
	})
}

func TestGetEmailVerified(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		userID   string
		cred     *domain.Credential
		credErr  error
		want     bool
		wantCode codes.Code // OK means no error
	}{
		{
			name:     "verified",
			userID:   "u1",
			cred:     &domain.Credential{UserID: "u1", EmailVerified: true},
			want:     true,
			wantCode: codes.OK,
		},
		{
			name:     "not verified",
			userID:   "u1",
			cred:     &domain.Credential{UserID: "u1", EmailVerified: false},
			want:     false,
			wantCode: codes.OK,
		},
		{
			name:     "empty user id",
			userID:   "  ",
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "credential missing",
			userID:   "ghost",
			credErr:  pkgerr.NotFound("credential not found"),
			wantCode: codes.NotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			creds := &mockCredRepo{
				GetByUserIDFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
					return tc.cred, tc.credErr
				},
			}
			uc := testAuthUCVerify(creds, &mockTokenRepo{}, &mockUserClient{}, nil, nil)

			got, err := uc.GetEmailVerified(ctx, tc.userID)
			if tc.wantCode == codes.OK {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
				return
			}
			require.Error(t, err)
			st, _ := status.FromError(err)
			assert.Equal(t, tc.wantCode, st.Code())
		})
	}
}

func TestRegister_PartnerRole_SendsVerification(t *testing.T) {
	ctx := context.Background()

	for _, role := range []string{"master", "venue_owner"} {
		role := role
		t.Run(role, func(t *testing.T) {
			creds := &mockCredRepo{
				CreateFunc: func(_ context.Context, _ *domain.Credential) error { return nil },
			}
			users := &mockUserClient{
				CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
					return &domain.UserRecord{ID: "partner-1", Email: in.Email, Role: in.Role}, nil
				},
			}
			mail := &mockVerifyMail{enabled: true}
			uc := testAuthUCVerify(creds, &mockTokenRepo{}, users, &mockEmailVerificationRepo{}, mail)

			_, err := uc.Register(ctx, RegisterInput{
				Email:    "partner@example.com",
				Password: "partner-pass1",
				Name:     "Partner",
				Role:     role,
			})
			require.NoError(t, err)
			assert.Equal(t, 1, mail.sendCalled, "partner registration must send a verification email")
			assert.Equal(t, "partner@example.com", mail.lastTo)
		})
	}
}

func TestRegister_UserRole_DoesNotSendVerification(t *testing.T) {
	ctx := context.Background()
	creds := &mockCredRepo{
		CreateFunc: func(_ context.Context, _ *domain.Credential) error { return nil },
	}
	users := &mockUserClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: "u1", Email: in.Email, Role: in.Role}, nil
		},
	}
	mail := &mockVerifyMail{enabled: true}
	uc := testAuthUCVerify(creds, &mockTokenRepo{}, users, &mockEmailVerificationRepo{}, mail)

	_, err := uc.Register(ctx, RegisterInput{
		Email:    "user@example.com",
		Password: "user-pass1",
		Name:     "User",
		Role:     "user",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, mail.sendCalled, "regular user registration must not send a verification email")
}
