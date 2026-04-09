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
