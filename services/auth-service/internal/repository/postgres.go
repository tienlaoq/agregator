package repository

import (
	"context"
	"errors"
	"time"

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

// DeleteByUserID is the compensating action for a failed Register saga step.
// It removes a credential row that was inserted before the downstream CreateUser
// call, restoring the local DB to a clean state so the user can retry registration.
func (r *CredentialRepo) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM credentials WHERE user_id = $1`, userID)
	return err
}

// GetByEmail looks up a credential by email address.
//
// The caller MUST normalise the email to lower(trim(...)) before calling this
// method — Register does it via validateRegisterInput, OAuthLogin does it at
// the top of the function. The WHERE clause matches on lower(trim(email)) so
// that it hits the functional unique index added in migration 005
// (idx_credentials_email_lower). Passing a pre-normalised value on the right-
// hand side avoids the double lower(trim($1)) call that would otherwise be
// needed for correctness and keeps the query plan simple.
func (r *CredentialRepo) GetByEmail(ctx context.Context, email string) (*domain.Credential, error) {
	const q = `SELECT id, user_id, email, COALESCE(password_hash, ''), email_verified, created_at
	             FROM credentials
	            WHERE lower(trim(email)) = $1`
	c := &domain.Credential{}
	err := r.pool.QueryRow(ctx, q, email).Scan(&c.ID, &c.UserID, &c.Email, &c.PasswordHash, &c.EmailVerified, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("credential not found")
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *CredentialRepo) GetByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	const q = `SELECT id, user_id, email, COALESCE(password_hash, ''), email_verified, created_at FROM credentials WHERE user_id = $1`
	c := &domain.Credential{}
	err := r.pool.QueryRow(ctx, q, userID).Scan(&c.ID, &c.UserID, &c.Email, &c.PasswordHash, &c.EmailVerified, &c.CreatedAt)
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
		INSERT INTO credentials (user_id, email, provider, provider_id, email_verified)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, cred.UserID, cred.Email, cred.Provider, cred.ProviderID, cred.EmailVerified).
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

// SetEmailVerified flips the email_verified flag for a credential row.
func (r *CredentialRepo) SetEmailVerified(ctx context.Context, userID string, verified bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE credentials SET email_verified = $2 WHERE user_id = $1`, userID, verified)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pkgerr.NotFound("credential not found")
	}
	return nil
}

// PromoteOAuthEmail updates a previously-empty email on an OAuth-only credential
// row. Only touches rows where email IS NULL or '' to avoid clobbering a
// legitimately set address that might have arrived via a concurrent request.
func (r *CredentialRepo) PromoteOAuthEmail(ctx context.Context, userID, email string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE credentials SET email = $2
		  WHERE user_id = $1 AND (email IS NULL OR email = '')`,
		userID, email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pkgerr.NotFound("credential not found or email already set")
	}
	return nil
}

// DeleteOrphanOAuthAccounts removes unverified-email OAuth-only credential rows
// (email = '', password_hash IS NULL) that are older than minAge and have no
// associated active refresh tokens. This prevents unbounded accumulation of
// accounts created when a provider sent EmailVerified=false.
//
// The JOIN against refresh_tokens uses the index on refresh_tokens.user_id
// (migration 004) so the NOT EXISTS subquery stays cheap even on large tables.
func (r *CredentialRepo) DeleteOrphanOAuthAccounts(ctx context.Context, minAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-minAge)
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM credentials
		 WHERE (email IS NULL OR email = '')
		   AND password_hash IS NULL
		   AND created_at < $1
		   AND NOT EXISTS (
		         SELECT 1 FROM refresh_tokens rt
		          WHERE rt.user_id = credentials.user_id
		            AND rt.expires_at > now()
		       )`,
		cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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

// ConsumeByHash atomically deletes and returns the refresh token row.
// Returns pkgerr.NotFound when no row matched (absent, expired+cleaned, or
// already consumed by a concurrent request).
func (r *RefreshTokenRepo) ConsumeByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	const q = `
		DELETE FROM refresh_tokens
		WHERE token_hash = $1
		RETURNING id, user_id, token_hash, expires_at, created_at`
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

// DeleteByHash deletes the refresh token with the given hash.
// It returns pkgerr.NotFound when no row matched — the caller uses this to
// distinguish a token that was already consumed (reuse attack) from one that
// never existed.
func (r *RefreshTokenRepo) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *RefreshTokenRepo) DeleteByHash(ctx context.Context, tokenHash string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pkgerr.NotFound("refresh token not found")
	}
	return nil
}
