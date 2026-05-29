package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/tienlao/agregator/services/user-service/internal/domain"
)

type UserUseCase struct {
	repo domain.UserRepository
}

func NewUserUseCase(repo domain.UserRepository) *UserUseCase {
	return &UserUseCase{repo: repo}
}

// normalizeEmail folds the address into the canonical form used by both
// storage and lookup: trimmed and lower-cased. Matches the functional unique
// index idx_users_email_lower and the WHERE clause in GetByEmail.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (uc *UserUseCase) Create(ctx context.Context, user *domain.User) error {
	user.Email = normalizeEmail(user.Email)
	if user.Role == "" {
		user.Role = "user"
	}
	// Validate role upfront — avoids a wasted DB roundtrip on bad input and
	// keeps domain.ErrInvalidRole reachable independently of the migration's
	// CHECK constraint naming (the previous repo-level catch was a string
	// match on pgErr.ConstraintName, fragile to constraint renames).
	if !domain.IsValidRole(user.Role) {
		return domain.ErrInvalidRole
	}
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	return uc.repo.Create(ctx, user)
}

func (uc *UserUseCase) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *UserUseCase) GetBatch(ctx context.Context, ids []string) (map[string]*domain.User, error) {
	return uc.repo.GetBatch(ctx, ids)
}

func (uc *UserUseCase) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return uc.repo.GetByEmail(ctx, normalizeEmail(email))
}

// Update reads the user, applies the non-nil patch fields, and writes back.
// Returns domain.ErrNotFound (propagated from GetByID) when the user does not
// exist; never returns (nil, nil). Any other error is a real failure.
func (uc *UserUseCase) Update(ctx context.Context, id string, name, phone, avatarURL, bio *string) (*domain.User, error) {
	user, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if name != nil {
		user.Name = *name
	}
	if phone != nil {
		user.Phone = *phone
	}
	if avatarURL != nil {
		user.AvatarURL = *avatarURL
	}
	if bio != nil {
		user.Bio = *bio
	}
	user.UpdatedAt = time.Now()

	if err := uc.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
