package repository

import "github.com/jackc/pgx/v5/pgxpool"

// PasswordResetRepo persists single-use password-reset tokens. The behaviour
// lives in the shared tokenRepo; this type only pins the backing table and the
// not-found message. Implements domain.TokenRepository.
type PasswordResetRepo struct {
	*tokenRepo
}

func NewPasswordResetRepo(pool *pgxpool.Pool) *PasswordResetRepo {
	return &PasswordResetRepo{&tokenRepo{
		pool:        pool,
		table:       "password_reset_tokens",
		notFoundMsg: "reset token invalid or expired",
	}}
}
