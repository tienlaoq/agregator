package domain

import (
	"context"
	"time"
)

type CredentialRepository interface {
	Create(ctx context.Context, cred *Credential) error
	GetByEmail(ctx context.Context, email string) (*Credential, error)
	GetByUserID(ctx context.Context, userID string) (*Credential, error)
	GetByProvider(ctx context.Context, provider, providerID string) (*Credential, error)
	CreateOAuth(ctx context.Context, cred *Credential) error
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	GetByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	DeleteByUserID(ctx context.Context, userID string) error
	DeleteByHash(ctx context.Context, tokenHash string) error
}

type PasswordResetRepository interface {
	InvalidateUnusedByUserID(ctx context.Context, userID string) error
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	ConsumeByTokenHash(ctx context.Context, tokenHash string) (userID string, err error)
}
