package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/user-service/internal/domain"
)

func stringPtr(s string) *string { return &s }

func TestCreate_Success(t *testing.T) {
	var captured *domain.User
	repo := &mockUserRepo{
		CreateFunc: func(ctx context.Context, user *domain.User) error {
			captured = user
			return nil
		},
	}
	uc := NewUserUseCase(repo)
	u := &domain.User{ID: "id-1", Email: "a@example.com", Name: "Alice"}
	err := uc.Create(context.Background(), u)
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.False(t, captured.CreatedAt.IsZero())
	assert.False(t, captured.UpdatedAt.IsZero())
	assert.WithinDuration(t, time.Now(), captured.CreatedAt, 2*time.Second)
	assert.WithinDuration(t, time.Now(), captured.UpdatedAt, 2*time.Second)
	assert.Equal(t, "user", captured.Role)
	assert.Equal(t, "id-1", captured.ID)
	assert.Equal(t, "a@example.com", captured.Email)
	assert.Equal(t, "Alice", captured.Name)
}

func TestCreate_DefaultRole(t *testing.T) {
	var captured *domain.User
	repo := &mockUserRepo{
		CreateFunc: func(ctx context.Context, user *domain.User) error {
			captured = user
			return nil
		},
	}
	uc := NewUserUseCase(repo)
	u := &domain.User{Email: "x@y.com", Role: ""}
	require.NoError(t, uc.Create(context.Background(), u))
	require.NotNil(t, captured)
	assert.Equal(t, "user", captured.Role)
}

func TestCreate_CustomRole(t *testing.T) {
	var captured *domain.User
	repo := &mockUserRepo{
		CreateFunc: func(ctx context.Context, user *domain.User) error {
			captured = user
			return nil
		},
	}
	uc := NewUserUseCase(repo)
	u := &domain.User{Email: "o@p.com", Role: "partner"}
	require.NoError(t, uc.Create(context.Background(), u))
	require.NotNil(t, captured)
	assert.Equal(t, "partner", captured.Role)
}

func TestGetByID_Found(t *testing.T) {
	want := &domain.User{ID: "u1", Email: "e@e.com", Name: "N"}
	repo := &mockUserRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.User, error) {
			assert.Equal(t, "u1", id)
			return want, nil
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.GetByID(context.Background(), "u1")
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestGetByID_NotFound(t *testing.T) {
	repo := &mockUserRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.User, error) {
			return nil, nil
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.GetByID(context.Background(), "missing")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUpdate_Success(t *testing.T) {
	prevUpdated := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	stored := &domain.User{
		ID: "u1", Email: "e@e.com", Name: "Old", Phone: "111",
		AvatarURL: "http://old", Bio: "old bio",
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: prevUpdated,
	}
	var updated *domain.User
	repo := &mockUserRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.User, error) {
			return stored, nil
		},
		UpdateFunc: func(ctx context.Context, user *domain.User) error {
			updated = user
			return nil
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.Update(context.Background(), "u1",
		stringPtr("NewName"), stringPtr("222"), stringPtr("http://new"), stringPtr("new bio"))
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, updated)
	assert.Same(t, got, updated)
	assert.Equal(t, "NewName", updated.Name)
	assert.Equal(t, "222", updated.Phone)
	assert.Equal(t, "http://new", updated.AvatarURL)
	assert.Equal(t, "new bio", updated.Bio)
	assert.Equal(t, "e@e.com", updated.Email)
	assert.Equal(t, "u1", updated.ID)
	assert.True(t, updated.UpdatedAt.After(prevUpdated))
}

func TestUpdate_PartialUpdate(t *testing.T) {
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	prevUpdated := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	stored := &domain.User{
		ID: "u1", Email: "e@e.com", Name: "OldName", Phone: "555",
		AvatarURL: "http://av", Bio: "unchanged bio",
		CreatedAt: created, UpdatedAt: prevUpdated,
	}
	var updated *domain.User
	repo := &mockUserRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.User, error) {
			return stored, nil
		},
		UpdateFunc: func(ctx context.Context, user *domain.User) error {
			updated = user
			return nil
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.Update(context.Background(), "u1", stringPtr("OnlyName"), nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Same(t, got, updated)
	assert.Equal(t, "OnlyName", updated.Name)
	assert.Equal(t, "555", updated.Phone)
	assert.Equal(t, "http://av", updated.AvatarURL)
	assert.Equal(t, "unchanged bio", updated.Bio)
	assert.True(t, updated.UpdatedAt.After(prevUpdated))
}

func TestUpdate_UserNotFound(t *testing.T) {
	var updateCalls int
	repo := &mockUserRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.User, error) {
			return nil, nil
		},
		UpdateFunc: func(ctx context.Context, user *domain.User) error {
			updateCalls++
			return nil
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.Update(context.Background(), "nope", stringPtr("x"), nil, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Equal(t, 0, updateCalls)
}
