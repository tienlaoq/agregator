package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
)

type PasswordResetRepo struct {
	pool *pgxpool.Pool
}

func NewPasswordResetRepo(pool *pgxpool.Pool) *PasswordResetRepo {
	return &PasswordResetRepo{pool: pool}
}

func (r *PasswordResetRepo) InvalidateUnusedByUserID(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL`, userID)
	return err
}

func (r *PasswordResetRepo) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	return err
}

func (r *PasswordResetRepo) ConsumeByTokenHash(ctx context.Context, tokenHash string) (string, error) {
	var uid string
	err := r.pool.QueryRow(ctx, `
		UPDATE password_reset_tokens
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`,
		tokenHash,
	).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pkgerr.NotFound("reset token invalid or expired")
	}
	if err != nil {
		return "", err
	}
	return uid, nil
}
