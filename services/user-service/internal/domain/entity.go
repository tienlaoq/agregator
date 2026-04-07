package domain

import (
	"errors"
	"time"
)

var (
	ErrEmailExists = errors.New("email already exists")
	ErrInvalidRole = errors.New("invalid role")
)

type User struct {
	ID        string
	Email     string
	Phone     string
	Name      string
	Role      string
	AvatarURL string
	Bio       string
	CreatedAt time.Time
	UpdatedAt time.Time
}
