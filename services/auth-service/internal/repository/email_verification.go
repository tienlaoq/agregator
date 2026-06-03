package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
)

// EmailVerificationRepo implements domain.EmailVerificationRepository backed by
// PostgreSQL. It is a near-twin of PasswordResetRepo: hashed single-use tokens
// with a TTL, consumed atomically via UPDATE … RETURNING.
type EmailVerificationRepo struct {
	pool *pgxpool.Pool
}

func NewEmailVerificationRepo(pool *pgxpool.Pool) *EmailVerificationRepo {
	return &EmailVerificationRepo{pool: pool}
}

func (r *EmailVerificationRepo) InvalidateUnusedByUserID(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM email_verification_tokens WHERE user_id = $1 AND used_at IS NULL`, userID)
	return err
}

func (r *EmailVerificationRepo) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO email_verification_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	return err
}

func (r *EmailVerificationRepo) ConsumeByTokenHash(ctx context.Context, tokenHash string) (string, error) {
	var uid string
	err := r.pool.QueryRow(ctx, `
		UPDATE email_verification_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`,
		tokenHash,
	).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pkgerr.NotFound("verification token invalid or expired")
	}
	if err != nil {
		return "", err
	}
	return uid, nil
}

func (r *EmailVerificationRepo) DeleteExpired(ctx context.Context) (int64, error) {
	// Remove rows that have expired OR been used — neither can ever be consumed again.
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM email_verification_tokens WHERE expires_at < now() OR used_at IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
