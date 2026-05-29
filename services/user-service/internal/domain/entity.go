package domain

import (
	"errors"
	"time"
)

// ErrInvalidRole is returned by the usecase layer when Create receives a role
// that is not in AllowedRoles. The repository never returns this error: roles
// are validated before the DB call, so a 23514 CHECK violation in production
// indicates the migration drifted from AllowedRoles.
var ErrInvalidRole = errors.New("invalid role")

// ErrNotFound is returned by every read path (GetByID, GetByEmail) and by
// Update when the target row does not exist. It is a sentinel so callers can
// use errors.Is to distinguish "missing row" from "database failure" without
// the older (nil, nil) convention that silently invited misses at every
// callsite. The delivery layer maps it to codes.NotFound.
var ErrNotFound = errors.New("user not found")

// AllowedRoles mirrors the CHECK constraint in migrations/001_init.up.sql.
// Keep these two lists in sync — adding a role here requires a new migration
// that widens the CHECK constraint.
var AllowedRoles = map[string]struct{}{
	"user":        {},
	"venue_owner": {},
	"master":      {},
	"admin":       {},
}

// IsValidRole reports whether role is one of the project-recognised roles.
// The empty string is NOT valid; callers that want to default to "user"
// must do so before calling this helper.
func IsValidRole(role string) bool {
	_, ok := AllowedRoles[role]
	return ok
}

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
