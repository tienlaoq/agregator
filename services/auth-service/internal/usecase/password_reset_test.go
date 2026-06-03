package usecase

import (
	"context"
	"strings"
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

func testAuthUCWithReset(
	tb testing.TB,
	creds *mockCredRepo,
	tokens *mockTokenRepo,
	users *mockUserClient,
	reset *mockPasswordResetRepo,
	mail *mockPasswordMail,
) *AuthUseCase {
	tb.Helper()
	if mail == nil {
		mail = &mockPasswordMail{}
	}
	if reset == nil {
		reset = &mockPasswordResetRepo{}
	}
	return NewAuthUseCase(
		creds, tokens,
		reset, mail, time.Hour,
		noopEmailVerificationRepo{}, noopVerifyMail{},
		24*time.Hour,
		users, testPrivKey, time.Hour, 24*time.Hour,
		nil, // partnerNotify: nil is safe — Enqueue is guarded by nil-check
		zerolog.Nop(),
	)
}

func TestRequestPasswordReset_sendsWhenPasswordSet(t *testing.T) {
	ctx := context.Background()
	hash, err := hashPassword("oldpass")
	require.NoError(t, err)

	var invalidated, created bool
	reset := &mockPasswordResetRepo{
		InvalidateUnusedByUserIDFunc: func(_ context.Context, uid string) error {
			invalidated = true
			assert.Equal(t, "user-1", uid)
			return nil
		},
		CreateFunc: func(_ context.Context, uid, th string, exp time.Time) error {
			created = true
			assert.Equal(t, "user-1", uid)
			assert.NotEmpty(t, th)
			assert.True(t, exp.After(time.Now()))
			return nil
		},
	}
	mail := &mockPasswordMail{enabled: true}

	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, email string) (*domain.Credential, error) {
			assert.Equal(t, "me@example.com", email)
			return &domain.Credential{UserID: "user-1", Email: "me@example.com", PasswordHash: hash}, nil
		},
	}

	uc := testAuthUCWithReset(t, creds, &mockTokenRepo{}, &mockUserClient{}, reset, mail)
	require.NoError(t, uc.RequestPasswordReset(ctx, "  Me@Example.COM  "))
	require.True(t, invalidated)
	require.True(t, created)
	require.Equal(t, 1, mail.sendCalled)
	assert.Equal(t, "me@example.com", mail.lastTo)
	assert.NotEmpty(t, mail.lastToken)
}

func TestRequestPasswordReset_scenarios(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		creds    *mockCredRepo
		email    string
		wantSend int
	}{
		{
			name: "unknown email yields no mail",
			creds: &mockCredRepo{
				GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
					return nil, pkgerr.NotFound("no")
				},
			},
			email:    "x@y.com",
			wantSend: 0,
		},
		{
			name: "oauth only yields no mail",
			creds: &mockCredRepo{
				GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
					return &domain.Credential{UserID: "u1", Email: "o@x.com", PasswordHash: ""}, nil
				},
			},
			email:    "o@x.com",
			wantSend: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mail := &mockPasswordMail{enabled: true}
			uc := testAuthUCWithReset(t, tt.creds, &mockTokenRepo{}, &mockUserClient{}, &mockPasswordResetRepo{}, mail)
			require.NoError(t, uc.RequestPasswordReset(ctx, tt.email))
			assert.Equal(t, tt.wantSend, mail.sendCalled)
		})
	}
}

func TestRequestPasswordReset_noOpWhenMailDisabled(t *testing.T) {
	ctx := context.Background()
	passHash, err := hashPassword("p")
	require.NoError(t, err)
	creds := &mockCredRepo{
		GetByEmailFunc: func(_ context.Context, _ string) (*domain.Credential, error) {
			return &domain.Credential{UserID: "u1", Email: "a@b.com", PasswordHash: passHash}, nil
		},
	}
	var created bool
	reset := &mockPasswordResetRepo{
		CreateFunc: func(_ context.Context, _, _ string, _ time.Time) error {
			created = true
			return nil
		},
	}
	mail := &mockPasswordMail{enabled: false}
	uc := testAuthUCWithReset(t, creds, &mockTokenRepo{}, &mockUserClient{}, reset, mail)
	require.NoError(t, uc.RequestPasswordReset(ctx, "a@b.com"))
	assert.False(t, created)
	assert.Equal(t, 0, mail.sendCalled)
}

func TestCompletePasswordReset_success(t *testing.T) {
	ctx := context.Background()
	raw := "reset-raw-token-xyz"
	th := hashRefreshToken(raw)

	var updatedUser string
	var newHash string
	creds := &mockCredRepo{
		UpdatePasswordHashFunc: func(_ context.Context, uid, ph string) error {
			updatedUser = uid
			newHash = ph
			return nil
		},
	}
	var deletedUser string
	tokens := &mockTokenRepo{
		DeleteByUserIDFunc: func(_ context.Context, uid string) error {
			deletedUser = uid
			return nil
		},
	}
	reset := &mockPasswordResetRepo{
		ConsumeByTokenHashFunc: func(_ context.Context, h string) (string, error) {
			assert.Equal(t, th, h)
			return "user-99", nil
		},
	}
	uc := testAuthUCWithReset(t, creds, tokens, &mockUserClient{}, reset, &mockPasswordMail{})
	require.NoError(t, uc.CompletePasswordReset(ctx, raw, "newpass123"))
	assert.Equal(t, "user-99", updatedUser)
	assert.Equal(t, "user-99", deletedUser)
	assert.NotEmpty(t, newHash)
	assert.True(t, verifyPassword("newpass123", newHash))
}

func TestCompletePasswordReset_invalidToken(t *testing.T) {
	ctx := context.Background()
	reset := &mockPasswordResetRepo{
		ConsumeByTokenHashFunc: func(_ context.Context, _ string) (string, error) {
			return "", pkgerr.NotFound("bad")
		},
	}
	uc := testAuthUCWithReset(t, &mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{}, reset, nil)
	err := uc.CompletePasswordReset(ctx, "badtoken", "newpass123")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}

func TestCompletePasswordReset_shortPassword(t *testing.T) {
	ctx := context.Background()
	uc := testAuthUCWithReset(t, &mockCredRepo{}, &mockTokenRepo{}, &mockUserClient{}, &mockPasswordResetRepo{}, nil)
	err := uc.CompletePasswordReset(ctx, "tok", "short")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.True(t, strings.Contains(st.Message(), "8"))
}

func TestCompletePasswordReset_secondSubmitFails(t *testing.T) {
	ctx := context.Background()
	raw := "same-token"
	th := hashRefreshToken(raw)
	calls := 0
	reset := &mockPasswordResetRepo{
		ConsumeByTokenHashFunc: func(_ context.Context, h string) (string, error) {
			assert.Equal(t, th, h)
			calls++
			if calls == 1 {
				return "user-1", nil
			}
			return "", pkgerr.NotFound("used")
		},
	}
	creds := &mockCredRepo{
		UpdatePasswordHashFunc: func(_ context.Context, _, _ string) error { return nil },
	}
	tokens := &mockTokenRepo{
		DeleteByUserIDFunc: func(_ context.Context, _ string) error { return nil },
	}
	uc := testAuthUCWithReset(t, creds, tokens, &mockUserClient{}, reset, nil)
	require.NoError(t, uc.CompletePasswordReset(ctx, raw, "longenough1"))
	err := uc.CompletePasswordReset(ctx, raw, "longenough2")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
}
