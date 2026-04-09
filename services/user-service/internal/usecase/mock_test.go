package usecase

import (
	"context"

	"github.com/tienlao/agregator/services/user-service/internal/domain"
)

// mockUserRepo implements domain.UserRepository for tests via function fields.
type mockUserRepo struct {
	CreateFunc     func(ctx context.Context, user *domain.User) error
	GetByIDFunc    func(ctx context.Context, id string) (*domain.User, error)
	GetByEmailFunc func(ctx context.Context, email string) (*domain.User, error)
	UpdateFunc     func(ctx context.Context, user *domain.User) error
}

func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, user)
	}
	return nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.GetByEmailFunc != nil {
		return m.GetByEmailFunc(ctx, email)
	}
	return nil, nil
}

func (m *mockUserRepo) Update(ctx context.Context, user *domain.User) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, user)
	}
	return nil
}
