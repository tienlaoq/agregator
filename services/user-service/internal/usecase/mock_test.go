package usecase

import (
	"context"

	"github.com/tienlao/agregator/services/user-service/internal/domain"
)

// mockUserRepo implements domain.UserRepository for tests via function fields.
type mockUserRepo struct {
	CreateFunc     func(ctx context.Context, user *domain.User) error
	GetByIDFunc    func(ctx context.Context, id string) (*domain.User, error)
	GetBatchFunc   func(ctx context.Context, ids []string) (map[string]*domain.User, error)
	GetByEmailFunc func(ctx context.Context, email string) (*domain.User, error)
	UpdateFunc     func(ctx context.Context, user *domain.User) error
	SoftDeleteFunc func(ctx context.Context, id string) error
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
	// Default mirrors the real repo contract: missing rows surface as
	// ErrNotFound, never (nil, nil). Tests that need a positive result must
	// set GetByIDFunc explicitly.
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) GetBatch(ctx context.Context, ids []string) (map[string]*domain.User, error) {
	if m.GetBatchFunc != nil {
		return m.GetBatchFunc(ctx, ids)
	}
	return map[string]*domain.User{}, nil
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.GetByEmailFunc != nil {
		return m.GetByEmailFunc(ctx, email)
	}
	// Default mirrors the real repo contract: missing rows surface as
	// ErrNotFound, never (nil, nil).
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) Update(ctx context.Context, user *domain.User) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, user)
	}
	return nil
}

func (m *mockUserRepo) SoftDelete(ctx context.Context, id string) error {
	if m.SoftDeleteFunc != nil {
		return m.SoftDeleteFunc(ctx, id)
	}
	return nil
}
