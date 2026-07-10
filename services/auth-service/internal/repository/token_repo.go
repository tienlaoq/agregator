package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
)

// tokenRepo is the shared implementation for single-use, hashed, TTL-bound
// tokens stored one row per token. Password-reset and email-verification are
// structurally identical (issue → invalidate previous → consume once → sweep
// expired) and differ only in their backing table and not-found message; both
// are supplied by the thin exported constructors in password_reset.go and
// email_verification.go.
//
// table is a compile-time constant chosen by those constructors — never user
// input — so interpolating it into the SQL text carries no injection risk (SQL
// identifiers cannot be bound parameters anyway). Every runtime value
// (user_id, token_hash, expires_at) stays parameterised via $1/$2/$3.
type tokenRepo struct {
	pool        *pgxpool.Pool
	table       string
	notFoundMsg string
}

func (r *tokenRepo) InvalidateUnusedByUserID(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM `+r.table+` WHERE user_id = $1 AND used_at IS NULL`, userID)
	return err
}

func (r *tokenRepo) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO `+r.table+` (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	return err
}

func (r *tokenRepo) DeleteExpired(ctx context.Context) (int64, error) {
	// Remove rows that have expired OR been used — neither can ever be consumed again.
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM `+r.table+` WHERE expires_at < now() OR used_at IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *tokenRepo) ConsumeByTokenHash(ctx context.Context, tokenHash string) (string, error) {
	var uid string
	err := r.pool.QueryRow(ctx,
		`UPDATE `+r.table+`
		SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`,
		tokenHash,
	).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", pkgerr.NotFound(r.notFoundMsg)
	}
	if err != nil {
		return "", err
	}
	return uid, nil
}
