package domain

import "time"

// CurrentConsentVersion identifies the 152-ФЗ consent text currently published
// on the site (/consent). Stamped onto new password registrations so we can
// prove which wording a user accepted. Bump this string whenever the consent
// page text changes.
// ponytail: single server-side version constant; if you need per-locale or
// draft/published states, move it to a table.
const CurrentConsentVersion = "2026-07-16"

type Credential struct {
	ID            string
	UserID        string
	Email         string
	PasswordHash  string
	Provider      string
	ProviderID    string
	EmailVerified bool
	// ConsentVersion is the accepted consent text version, or "" for rows that
	// predate consent tracking / OAuth sign-ups without a consent checkbox.
	ConsentVersion string
	CreatedAt      time.Time
}

type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type PasswordResetToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

type EmailVerificationToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// UserRecord is the subset of user-service data that auth-service needs.
// Keeping it here prevents the usecase layer from importing gen/go/user/v1.
type UserRecord struct {
	ID    string
	Email string
	Role  string
}

// CreateUserInput carries the fields auth-service passes to user-service on
// registration. Only the fields actually used by auth-service are listed.
type CreateUserInput struct {
	ID    string
	Email string
	Phone string
	Name  string
	Role  string
}
