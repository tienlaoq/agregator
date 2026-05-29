package usecase

import (
	"context"
	"errors"
	"time"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"
)

type mockCredRepo struct {
	CreateFunc                    func(ctx context.Context, cred *domain.Credential) error
	DeleteByUserIDFunc            func(ctx context.Context, userID string) error
	GetByEmailFunc                func(ctx context.Context, email string) (*domain.Credential, error)
	GetByUserIDFunc               func(ctx context.Context, userID string) (*domain.Credential, error)
	GetByProviderFunc             func(ctx context.Context, provider, providerID string) (*domain.Credential, error)
	CreateOAuthFunc               func(ctx context.Context, cred *domain.Credential) error
	UpdatePasswordHashFunc        func(ctx context.Context, userID, passwordHash string) error
	PromoteOAuthEmailFunc         func(ctx context.Context, userID, email string) error
	DeleteOrphanOAuthAccountsFunc func(ctx context.Context, minAge time.Duration) (int64, error)
}

func (m *mockCredRepo) Create(ctx context.Context, cred *domain.Credential) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, cred)
	}
	return nil
}

func (m *mockCredRepo) DeleteByUserID(ctx context.Context, userID string) error {
	if m.DeleteByUserIDFunc != nil {
		return m.DeleteByUserIDFunc(ctx, userID)
	}
	return nil
}

func (m *mockCredRepo) GetByEmail(ctx context.Context, email string) (*domain.Credential, error) {
	if m.GetByEmailFunc != nil {
		return m.GetByEmailFunc(ctx, email)
	}
	return nil, pkgerr.NotFound("credential not found")
}

func (m *mockCredRepo) GetByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	if m.GetByUserIDFunc != nil {
		return m.GetByUserIDFunc(ctx, userID)
	}
	return nil, pkgerr.NotFound("credential not found")
}

func (m *mockCredRepo) GetByProvider(ctx context.Context, provider, providerID string) (*domain.Credential, error) {
	if m.GetByProviderFunc != nil {
		return m.GetByProviderFunc(ctx, provider, providerID)
	}
	return nil, pkgerr.NotFound("credential not found")
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

func (m *mockCredRepo) PromoteOAuthEmail(ctx context.Context, userID, email string) error {
	if m.PromoteOAuthEmailFunc != nil {
		return m.PromoteOAuthEmailFunc(ctx, userID, email)
	}
	return nil
}

func (m *mockCredRepo) DeleteOrphanOAuthAccounts(ctx context.Context, minAge time.Duration) (int64, error) {
	if m.DeleteOrphanOAuthAccountsFunc != nil {
		return m.DeleteOrphanOAuthAccountsFunc(ctx, minAge)
	}
	return 0, nil
}

type mockTokenRepo struct {
	CreateFunc         func(ctx context.Context, token *domain.RefreshToken) error
	ConsumeByHashFunc  func(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
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

// ConsumeByHash is the primary path used by RefreshToken (DELETE … RETURNING).
func (m *mockTokenRepo) ConsumeByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	if m.ConsumeByHashFunc != nil {
		return m.ConsumeByHashFunc(ctx, tokenHash)
	}
	return nil, pkgerr.NotFound("refresh token not found")
}

// GetByHash is kept for tests that exercise the expiry-cleanup path or other
// callers that still need a read-only lookup.
func (m *mockTokenRepo) GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	if m.GetByHashFunc != nil {
		return m.GetByHashFunc(ctx, tokenHash)
	}
	return nil, pkgerr.NotFound("refresh token not found")
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

func (m *mockTokenRepo) DeleteExpired(_ context.Context) (int64, error) { return 0, nil }

// mockUserClient implements domain.UserClient for tests.
// Only the two methods used by AuthUseCase are needed.
type mockUserClient struct {
	CreateUserFunc func(ctx context.Context, in domain.CreateUserInput) (*domain.UserRecord, error)
	GetUserFunc    func(ctx context.Context, userID string) (*domain.UserRecord, error)
}

func (m *mockUserClient) CreateUser(ctx context.Context, in domain.CreateUserInput) (*domain.UserRecord, error) {
	if m.CreateUserFunc != nil {
		return m.CreateUserFunc(ctx, in)
	}
	return nil, nil
}

func (m *mockUserClient) GetUser(ctx context.Context, userID string) (*domain.UserRecord, error) {
	if m.GetUserFunc != nil {
		return m.GetUserFunc(ctx, userID)
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

func (m *mockPasswordResetRepo) DeleteExpired(_ context.Context) (int64, error) { return 0, nil }

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
func (noopPasswordResetRepo) Create(context.Context, string, string, time.Time) error { return nil }
func (noopPasswordResetRepo) ConsumeByTokenHash(context.Context, string) (string, error) {
	return "", errors.New("noop reset repo")
}
func (noopPasswordResetRepo) DeleteExpired(context.Context) (int64, error) { return 0, nil }

type noopPasswordMail struct{}

func (noopPasswordMail) Enabled() bool { return false }
func (noopPasswordMail) SendPasswordReset(context.Context, string, string) error {
	return nil
}

// mockPartnerNotifier records Enqueue calls for assertions in tests.
type mockPartnerNotifier struct {
	events []domain.PartnerRegisteredEvent
}

func (m *mockPartnerNotifier) Enqueue(ev domain.PartnerRegisteredEvent) {
	m.events = append(m.events, ev)
}
