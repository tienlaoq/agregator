package usecase

import (
	"context"
	"time"

	"github.com/tienlao/agregator/services/user-service/internal/domain"
)

type UserUseCase struct {
	repo domain.UserRepository
}

func NewUserUseCase(repo domain.UserRepository) *UserUseCase {
	return &UserUseCase{repo: repo}
}

func (uc *UserUseCase) Create(ctx context.Context, user *domain.User) error {
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now
	if user.Role == "" {
		user.Role = "user"
	}
	return uc.repo.Create(ctx, user)
}

func (uc *UserUseCase) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return uc.repo.GetByID(ctx, id)
}

func (uc *UserUseCase) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return uc.repo.GetByEmail(ctx, email)
}

func (uc *UserUseCase) Update(ctx context.Context, id string, name, phone, avatarURL, bio *string) (*domain.User, error) {
	user, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
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
