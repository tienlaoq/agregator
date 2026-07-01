package grpc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	authv1 "github.com/tienlao/agregator/gen/go/auth/v1"
	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
	"github.com/tienlao/agregator/services/auth-service/internal/usecase"
)

// --- domain mocks (function-field style, mirroring usecase/mock_test.go) ---

type credRepo struct {
	CreateFunc      func(ctx context.Context, c *domain.Credential) error
	GetByEmailFunc  func(ctx context.Context, email string) (*domain.Credential, error)
	GetByUserIDFunc func(ctx context.Context, userID string) (*domain.Credential, error)
	DeleteFunc      func(ctx context.Context, userID string) error
}

func (r *credRepo) Create(ctx context.Context, c *domain.Credential) error {
	if r.CreateFunc != nil {
		return r.CreateFunc(ctx, c)
	}
	return nil
}
func (r *credRepo) DeleteByUserID(ctx context.Context, userID string) error {
	if r.DeleteFunc != nil {
		return r.DeleteFunc(ctx, userID)
	}
	return nil
}
func (r *credRepo) GetByEmail(ctx context.Context, email string) (*domain.Credential, error) {
	if r.GetByEmailFunc != nil {
		return r.GetByEmailFunc(ctx, email)
	}
	return nil, pkgerr.NotFound("not found")
}
func (r *credRepo) GetByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	if r.GetByUserIDFunc != nil {
		return r.GetByUserIDFunc(ctx, userID)
	}
	return nil, pkgerr.NotFound("not found")
}
func (r *credRepo) GetByProvider(context.Context, string, string) (*domain.Credential, error) {
	return nil, pkgerr.NotFound("not found")
}
func (r *credRepo) CreateOAuth(context.Context, *domain.Credential) error    { return nil }
func (r *credRepo) UpdatePasswordHash(context.Context, string, string) error { return nil }
func (r *credRepo) SetEmailVerified(context.Context, string, bool) error     { return nil }
func (r *credRepo) PromoteOAuthEmail(context.Context, string, string) error  { return nil }
func (r *credRepo) DeleteOrphanOAuthAccounts(context.Context, time.Duration) (int64, error) {
	return 0, nil
}

type tokenRepo struct {
	CreateFunc         func(ctx context.Context, t *domain.RefreshToken) error
	DeleteByUserIDFunc func(ctx context.Context, userID string) error
	DeleteByHashFunc   func(ctx context.Context, hash string) error
}

func (r *tokenRepo) Create(ctx context.Context, t *domain.RefreshToken) error {
	if r.CreateFunc != nil {
		return r.CreateFunc(ctx, t)
	}
	return nil
}
func (r *tokenRepo) ConsumeByHash(context.Context, string) (*domain.RefreshToken, error) {
	return nil, pkgerr.NotFound("not found")
}
func (r *tokenRepo) GetByHash(context.Context, string) (*domain.RefreshToken, error) {
	return nil, pkgerr.NotFound("not found")
}
func (r *tokenRepo) DeleteByUserID(ctx context.Context, userID string) error {
	if r.DeleteByUserIDFunc != nil {
		return r.DeleteByUserIDFunc(ctx, userID)
	}
	return nil
}
func (r *tokenRepo) DeleteByHash(ctx context.Context, hash string) error {
	if r.DeleteByHashFunc != nil {
		return r.DeleteByHashFunc(ctx, hash)
	}
	return nil
}
func (r *tokenRepo) DeleteExpired(context.Context) (int64, error) { return 0, nil }

type userClient struct {
	CreateUserFunc func(ctx context.Context, in domain.CreateUserInput) (*domain.UserRecord, error)
	GetUserFunc    func(ctx context.Context, userID string) (*domain.UserRecord, error)
}

func (u *userClient) CreateUser(ctx context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
	if u.CreateUserFunc != nil {
		return u.CreateUserFunc(ctx, in)
	}
	return &domain.UserRecord{ID: in.ID, Email: in.Email, Role: in.Role}, nil
}
func (u *userClient) GetUser(ctx context.Context, userID string) (*domain.UserRecord, error) {
	if u.GetUserFunc != nil {
		return u.GetUserFunc(ctx, userID)
	}
	return &domain.UserRecord{ID: userID}, nil
}

type noopResetRepo struct{}

func (noopResetRepo) InvalidateUnusedByUserID(context.Context, string) error  { return nil }
func (noopResetRepo) Create(context.Context, string, string, time.Time) error { return nil }
func (noopResetRepo) ConsumeByTokenHash(context.Context, string) (string, error) {
	return "", pkgerr.NotFound("not found")
}
func (noopResetRepo) DeleteExpired(context.Context) (int64, error) { return 0, nil }

type noopVerifyRepo struct{}

func (noopVerifyRepo) InvalidateUnusedByUserID(context.Context, string) error  { return nil }
func (noopVerifyRepo) Create(context.Context, string, string, time.Time) error { return nil }
func (noopVerifyRepo) ConsumeByTokenHash(context.Context, string) (string, error) {
	return "", pkgerr.NotFound("not found")
}
func (noopVerifyRepo) DeleteExpired(context.Context) (int64, error) { return 0, nil }

type noopMailer struct{}

func (noopMailer) Enabled() bool                                           { return false }
func (noopMailer) SendPasswordReset(context.Context, string, string) error { return nil }
func (noopMailer) SendVerification(context.Context, string, string) error  { return nil }

var testKey *ecdsa.PrivateKey

func init() {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic("server_test: generate key: " + err.Error())
	}
	testKey = k
}

func newServer(creds *credRepo, tokens *tokenRepo, users *userClient) *Server {
	uc := usecase.NewAuthUseCase(
		creds, tokens,
		noopResetRepo{}, noopMailer{},
		time.Hour,
		noopVerifyRepo{}, noopMailer{},
		24*time.Hour,
		users, testKey, time.Hour, 24*time.Hour,
		nil,
		zerolog.Nop(),
	)
	return NewServer(uc, zerolog.Nop())
}

func TestRegister_MapsResponse(t *testing.T) {
	creds := &credRepo{}
	tokens := &tokenRepo{}
	users := &userClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: in.ID, Email: in.Email, Role: in.Role}, nil
		},
	}
	s := newServer(creds, tokens, users)

	resp, err := s.Register(context.Background(), &authv1.RegisterRequest{
		Email:    "reg@example.com",
		Phone:    "+1000",
		Password: "register-pass1",
		Name:     "Reg User",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if resp.GetUserId() == "" {
		t.Error("UserId not propagated to response")
	}
	if resp.GetAccessToken() == "" || resp.GetRefreshToken() == "" {
		t.Error("tokens not propagated to response")
	}
}

func TestRegister_PropagatesUsecaseError(t *testing.T) {
	users := &userClient{
		CreateUserFunc: func(context.Context, domain.CreateUserInput) (*domain.UserRecord, error) {
			return nil, errors.New("user-service down")
		},
	}
	s := newServer(&credRepo{}, &tokenRepo{}, users)

	_, err := s.Register(context.Background(), &authv1.RegisterRequest{
		Email:    "reg@example.com",
		Password: "register-pass1",
		Name:     "Reg",
		Role:     "user",
	})
	if err == nil {
		t.Fatal("expected error to propagate from usecase, got nil")
	}
}

func TestLogin_PropagatesError(t *testing.T) {
	// Unknown email → usecase returns Unauthenticated; handler must surface it.
	s := newServer(&credRepo{}, &tokenRepo{}, &userClient{})

	_, err := s.Login(context.Background(), &authv1.LoginRequest{
		Email:    "nobody@example.com",
		Password: "whatever-pass1",
	})
	if err == nil {
		t.Fatal("expected error for unknown account, got nil")
	}
}

func TestValidateToken_InvalidReturnsValidFalse(t *testing.T) {
	s := newServer(&credRepo{}, &tokenRepo{}, &userClient{})

	resp, err := s.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken: "not-a-real-jwt",
	})
	// Hot-path contract: bad token is NOT a gRPC error — it is Valid:false.
	if err != nil {
		t.Fatalf("ValidateToken must not return an error for a bad token, got %v", err)
	}
	if resp.GetValid() {
		t.Error("invalid token reported as Valid:true")
	}
}

func TestValidateToken_ValidEchoesClaims(t *testing.T) {
	users := &userClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: in.ID, Email: in.Email, Role: in.Role}, nil
		},
	}
	s := newServer(&credRepo{}, &tokenRepo{}, users)

	reg, err := s.Register(context.Background(), &authv1.RegisterRequest{
		Email:    "claims@example.com",
		Password: "register-pass1",
		Name:     "Claims User",
		Role:     "user",
	})
	if err != nil {
		t.Fatalf("setup Register failed: %v", err)
	}

	resp, err := s.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken: reg.GetAccessToken(),
	})
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}
	if !resp.GetValid() {
		t.Fatal("freshly issued token reported as invalid")
	}
	if resp.GetUserId() != reg.GetUserId() {
		t.Errorf("UserId = %q, want %q", resp.GetUserId(), reg.GetUserId())
	}
	if resp.GetEmail() != "claims@example.com" {
		t.Errorf("Email = %q, want claims@example.com", resp.GetEmail())
	}
	if resp.GetRole() != "user" {
		t.Errorf("Role = %q, want user", resp.GetRole())
	}
}

func TestDeleteAccount_Success(t *testing.T) {
	var tokDel, credDel bool
	creds := &credRepo{DeleteFunc: func(context.Context, string) error { credDel = true; return nil }}
	tokens := &tokenRepo{DeleteByUserIDFunc: func(context.Context, string) error { tokDel = true; return nil }}
	s := newServer(creds, tokens, &userClient{})

	_, err := s.DeleteAccount(context.Background(), &authv1.DeleteAccountRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("DeleteAccount error: %v", err)
	}
	if !tokDel || !credDel {
		t.Errorf("expected both token and credential deletes; tokDel=%v credDel=%v", tokDel, credDel)
	}
}

func TestDeleteAccount_EmptyUserID(t *testing.T) {
	s := newServer(&credRepo{}, &tokenRepo{}, &userClient{})
	_, err := s.DeleteAccount(context.Background(), &authv1.DeleteAccountRequest{UserId: ""})
	if err == nil {
		t.Fatal("expected InvalidArgument for empty user_id, got nil")
	}
}

func TestGetEmailVerification_ReturnsFlag(t *testing.T) {
	creds := &credRepo{
		GetByUserIDFunc: func(_ context.Context, userID string) (*domain.Credential, error) {
			return &domain.Credential{UserID: userID, EmailVerified: true}, nil
		},
	}
	s := newServer(creds, &tokenRepo{}, &userClient{})

	resp, err := s.GetEmailVerification(context.Background(), &authv1.GetEmailVerificationRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("GetEmailVerification error: %v", err)
	}
	if !resp.GetEmailVerified() {
		t.Error("expected EmailVerified:true to be propagated")
	}
}

func TestRefreshToken_PropagatesError(t *testing.T) {
	// Default ConsumeByHash returns NotFound → usecase surfaces an error.
	s := newServer(&credRepo{}, &tokenRepo{}, &userClient{})
	if _, err := s.RefreshToken(context.Background(), &authv1.RefreshTokenRequest{RefreshToken: "x"}); err == nil {
		t.Fatal("expected error for unknown refresh token, got nil")
	}
}

func TestCompletePasswordReset_PropagatesError(t *testing.T) {
	// Default reset repo ConsumeByTokenHash returns NotFound → error.
	s := newServer(&credRepo{}, &tokenRepo{}, &userClient{})
	_, err := s.CompletePasswordReset(context.Background(), &authv1.CompletePasswordResetRequest{
		Token:       "bad",
		NewPassword: "brand-new-pass1",
	})
	if err == nil {
		t.Fatal("expected error for invalid reset token, got nil")
	}
}

func TestVerifyEmail_PropagatesError(t *testing.T) {
	// Default verify repo ConsumeByTokenHash returns NotFound → error.
	s := newServer(&credRepo{}, &tokenRepo{}, &userClient{})
	if _, err := s.VerifyEmail(context.Background(), &authv1.VerifyEmailRequest{Token: "bad"}); err == nil {
		t.Fatal("expected error for invalid verification token, got nil")
	}
}

func TestRequestPasswordReset_SilentForUnknownEmail(t *testing.T) {
	// Anti-enumeration: unknown email must return an empty success, not an error.
	s := newServer(&credRepo{}, &tokenRepo{}, &userClient{})
	if _, err := s.RequestPasswordReset(context.Background(), &authv1.RequestPasswordResetRequest{
		Email: "nobody@example.com",
	}); err != nil {
		t.Fatalf("RequestPasswordReset must not leak account existence via error, got %v", err)
	}
}

func TestResendVerification_SilentForUnknownEmail(t *testing.T) {
	s := newServer(&credRepo{}, &tokenRepo{}, &userClient{})
	if _, err := s.ResendVerification(context.Background(), &authv1.ResendVerificationRequest{
		Email: "nobody@example.com",
	}); err != nil {
		t.Fatalf("ResendVerification must not leak account existence via error, got %v", err)
	}
}

func TestOAuthLogin_NewUser(t *testing.T) {
	// No existing credential by provider or email → a brand-new OAuth account is
	// created and IsNewUser must be reported as true.
	users := &userClient{
		CreateUserFunc: func(_ context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
			return &domain.UserRecord{ID: in.ID, Email: in.Email, Role: in.Role}, nil
		},
	}
	s := newServer(&credRepo{}, &tokenRepo{}, users)

	resp, err := s.OAuthLogin(context.Background(), &authv1.OAuthLoginRequest{
		Provider:      "google",
		ProviderId:    "g-123",
		Email:         "oauth@example.com",
		Name:          "OAuth User",
		EmailVerified: true,
	})
	if err != nil {
		t.Fatalf("OAuthLogin error: %v", err)
	}
	if !resp.GetIsNewUser() {
		t.Error("expected IsNewUser:true for a first-time OAuth login")
	}
	if resp.GetAccessToken() == "" || resp.GetRefreshToken() == "" {
		t.Error("tokens not propagated to OAuth response")
	}
}

func TestLogout_Idempotent(t *testing.T) {
	// DeleteByHash returning NotFound must be swallowed → no error.
	tokens := &tokenRepo{
		DeleteByHashFunc: func(context.Context, string) error { return pkgerr.NotFound("gone") },
	}
	s := newServer(&credRepo{}, tokens, &userClient{})

	if _, err := s.Logout(context.Background(), &authv1.LogoutRequest{RefreshToken: "x"}); err != nil {
		t.Fatalf("Logout should be idempotent on missing token, got %v", err)
	}
}
