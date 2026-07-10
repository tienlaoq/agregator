package repository

import "github.com/jackc/pgx/v5/pgxpool"

// EmailVerificationRepo persists single-use email-verification tokens. The
// behaviour lives in the shared tokenRepo; this type only pins the backing
// table and the not-found message. Implements domain.TokenRepository.
type EmailVerificationRepo struct {
	*tokenRepo
}

func NewEmailVerificationRepo(pool *pgxpool.Pool) *EmailVerificationRepo {
	return &EmailVerificationRepo{&tokenRepo{
		pool:        pool,
		table:       "email_verification_tokens",
		notFoundMsg: "verification token invalid or expired",
	}}
}
