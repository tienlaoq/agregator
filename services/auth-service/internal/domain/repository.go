package domain

import (
	"context"
	"time"
)

type CredentialRepository interface {
	Create(ctx context.Context, cred *Credential) error
	// DeleteByUserID removes a credential row. Used as a compensating action when the
	// downstream CreateUser call fails after a credential was already inserted locally.
	DeleteByUserID(ctx context.Context, userID string) error
	GetByEmail(ctx context.Context, email string) (*Credential, error)
	GetByUserID(ctx context.Context, userID string) (*Credential, error)
	GetByProvider(ctx context.Context, provider, providerID string) (*Credential, error)
	CreateOAuth(ctx context.Context, cred *Credential) error
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error

	// SetEmailVerified flips the email_verified flag for a credential row.
	// Used by VerifyEmail (true) once a verification token is consumed and by
	// OAuthLogin (true) since the provider already vouched for the address.
	// Returns pkgerr.NotFound when no row matched.
	SetEmailVerified(ctx context.Context, userID string, verified bool) error

	// PromoteOAuthEmail sets the email on a credential row that was created with
	// an empty email (EmailVerified=false path in OAuthLogin). Called when the
	// same provider later returns EmailVerified=true, allowing the orphan account
	// to become a full account without re-registration.
	// Returns pkgerr.NotFound if no matching row exists.
	PromoteOAuthEmail(ctx context.Context, userID, email string) error

	// DeleteOrphanOAuthAccounts removes credential rows that have no email and no
	// password hash (pure unverified-OAuth accounts) and were created more than
	// minAge ago and have had no active refresh tokens since creation.
	// Called periodically by the cleanup goroutine. Returns the count of deleted rows.
	DeleteOrphanOAuthAccounts(ctx context.Context, minAge time.Duration) (int64, error)
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error

	// ConsumeByHash atomically deletes the row with the given hash and returns
	// it. This is the single entry-point for both validation and revocation in
	// RefreshToken: a DELETE … RETURNING avoids a separate SELECT that would
	// expose whether a user_id exists before the token is verified.
	//
	// Returns pkgerr.NotFound when no row matched (token absent, already expired
	// and cleaned up, or already consumed by a concurrent request).
	//
	// Note on reuse-detection: if an attacker stole a token, used it before the
	// legitimate owner, and the owner's retry arrives after the attacker's request
	// completed, GetByHash would return NotFound — the same result as a plain
	// invalid token. Full family-invalidation on delayed replay requires a
	// tombstone table (see docs/TECH_DEBT.md [AUTH-REUSE-TOMBSTONE]).
	// ConsumeByHash still catches the concurrent-request race: if two requests
	// arrive in the same window only one DELETE will match, the other gets
	// NotFound → Unauthenticated, and that is safe.
	ConsumeByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)

	DeleteByUserID(ctx context.Context, userID string) error

	// DeleteByHash is kept for the expiry-cleanup path in RefreshToken where the
	// token is known to be expired and we just want to tidy the row without
	// needing the full record back.
	DeleteByHash(ctx context.Context, tokenHash string) error

	// DeleteExpired removes all rows whose expires_at is in the past.
	// Called periodically by the cleanup goroutine in cmd/main.go.
	DeleteExpired(ctx context.Context) (deleted int64, err error)
}

// TokenRepository manages single-use, hashed, TTL-bound tokens stored one row
// per token. Password-reset and email-verification share this contract: their
// only differences (backing table, not-found message) live in the concrete
// implementation, not the interface. Anti-enumeration is handled in the usecase
// layer, not here. The usecase holds a separate value per token kind, so each
// still points at its own table.
type TokenRepository interface {
	// InvalidateUnusedByUserID removes any outstanding (unused) tokens for the
	// user before a fresh one is issued, so only the latest link is ever valid.
	InvalidateUnusedByUserID(ctx context.Context, userID string) error
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	// ConsumeByTokenHash atomically marks the token used and returns its user_id.
	// Returns pkgerr.NotFound when the token is absent, expired, or already used.
	ConsumeByTokenHash(ctx context.Context, tokenHash string) (userID string, err error)
	// DeleteExpired removes rows that have expired or been used.
	// Called periodically by the cleanup goroutine in cmd/main.go.
	DeleteExpired(ctx context.Context) (deleted int64, err error)
}

// UserClient is the port through which auth-service talks to user-service.
// Defined here so the usecase layer depends only on domain, not on the
// generated gRPC package (gen/go/user/v1).
// The adapter that implements this interface lives in internal/adapter/userclient.
type UserClient interface {
	CreateUser(ctx context.Context, in CreateUserInput) (*UserRecord, error)
	GetUser(ctx context.Context, userID string) (*UserRecord, error)
}
