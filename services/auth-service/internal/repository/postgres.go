package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tienlao/agregator/services/auth-service/internal/domain"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
)

// CredentialRepo implements domain.CredentialRepository backed by PostgreSQL.
type CredentialRepo struct {
	pool *pgxpool.Pool
}

func NewCredentialRepo(pool *pgxpool.Pool) *CredentialRepo {
	return &CredentialRepo{pool: pool}
}

func (r *CredentialRepo) Create(ctx context.Context, cred *domain.Credential) error {
	const q = `
		INSERT INTO credentials (user_id, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, cred.UserID, cred.Email, cred.PasswordHash).
		Scan(&cred.ID, &cred.CreatedAt)
}

func (r *CredentialRepo) GetByEmail(ctx context.Context, email string) (*domain.Credential, error) {
	const q = `SELECT id, user_id, email, COALESCE(password_hash, ''), created_at FROM credentials WHERE lower(trim(email)) = lower(trim($1))`
	c := &domain.Credential{}
	err := r.pool.QueryRow(ctx, q, email).Scan(&c.ID, &c.UserID, &c.Email, &c.PasswordHash, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("credential not found")
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *CredentialRepo) GetByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	const q = `SELECT id, user_id, email, COALESCE(password_hash, ''), created_at FROM credentials WHERE user_id = $1`
	c := &domain.Credential{}
	err := r.pool.QueryRow(ctx, q, userID).Scan(&c.ID, &c.UserID, &c.Email, &c.PasswordHash, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("credential not found")
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *CredentialRepo) GetByProvider(ctx context.Context, provider, providerID string) (*domain.Credential, error) {
	const q = `SELECT id, user_id, email, COALESCE(password_hash, ''), provider, provider_id, created_at 
		FROM credentials WHERE provider = $1 AND provider_id = $2`
	c := &domain.Credential{}
	err := r.pool.QueryRow(ctx, q, provider, providerID).
		Scan(&c.ID, &c.UserID, &c.Email, &c.PasswordHash, &c.Provider, &c.ProviderID, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("credential not found")
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *CredentialRepo) CreateOAuth(ctx context.Context, cred *domain.Credential) error {
	const q = `
		INSERT INTO credentials (user_id, email, provider, provider_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, cred.UserID, cred.Email, cred.Provider, cred.ProviderID).
		Scan(&cred.ID, &cred.CreatedAt)
}

func (r *CredentialRepo) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE credentials SET password_hash = $2 WHERE user_id = $1`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pkgerr.NotFound("credential not found")
	}
	return nil
}

// RefreshTokenRepo implements domain.RefreshTokenRepository backed by PostgreSQL.
type RefreshTokenRepo struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepo(pool *pgxpool.Pool) *RefreshTokenRepo {
	return &RefreshTokenRepo{pool: pool}
}

func (r *RefreshTokenRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	const q = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, token.UserID, token.TokenHash, token.ExpiresAt).
		Scan(&token.ID, &token.CreatedAt)
}

func (r *RefreshTokenRepo) GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	const q = `SELECT id, user_id, token_hash, expires_at, created_at FROM refresh_tokens WHERE token_hash = $1`
	t := &domain.RefreshToken{}
	err := r.pool.QueryRow(ctx, q, tokenHash).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("refresh token not found")
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *RefreshTokenRepo) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	return err
}

func (r *RefreshTokenRepo) DeleteByHash(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, tokenHash)
	return err
}
