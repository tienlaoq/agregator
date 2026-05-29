package domain

import "time"

type Credential struct {
	ID           string
	UserID       string
	Email        string
	PasswordHash string
	Provider     string
	ProviderID   string
	CreatedAt    time.Time
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
