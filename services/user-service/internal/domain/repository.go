package domain

import "context"

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	// GetByID returns ErrNotFound if no row matches; never returns (nil, nil).
	GetByID(ctx context.Context, id string) (*User, error)
	// GetBatch fetches multiple users by id in a single query. Missing ids are
	// silently omitted from the result map — callers must not treat absence as
	// an error. ErrNotFound is NOT returned for partial misses; a fully empty
	// result for a non-empty id list still returns (empty map, nil).
	GetBatch(ctx context.Context, ids []string) (map[string]*User, error)
	// GetByEmail returns ErrNotFound if no row matches; never returns (nil, nil).
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	// SoftDelete anonymises and deactivates a user in a single UPDATE: email is
	// rewritten to a unique tombstone (freeing the original address for re-use),
	// name/phone/avatar/bio are cleared and deleted_at is stamped. Returns
	// ErrNotFound when no active row matches (already deleted or never existed),
	// which makes the operation idempotent from the caller's perspective.
	SoftDelete(ctx context.Context, id string) error
}
