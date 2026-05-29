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

func TestCreate_NormalizesEmail(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"already_canonical", "alice@example.com", "alice@example.com"},
		{"uppercase", "ALICE@EXAMPLE.COM", "alice@example.com"},
		{"mixed_case", "Alice@Example.Com", "alice@example.com"},
		{"surrounding_whitespace", "  alice@example.com\t", "alice@example.com"},
		{"mixed_case_and_whitespace", "  Alice@EXAMPLE.com ", "alice@example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured *domain.User
			repo := &mockUserRepo{
				CreateFunc: func(ctx context.Context, user *domain.User) error {
					captured = user
					return nil
				},
			}
			uc := NewUserUseCase(repo)
			u := &domain.User{ID: "id-1", Email: tc.input, Name: "Alice"}
			require.NoError(t, uc.Create(context.Background(), u))
			require.NotNil(t, captured)
			assert.Equal(t, tc.want, captured.Email)
			// Caller's struct is also mutated — the canonical form is what
			// gets persisted and what subsequent code should observe.
			assert.Equal(t, tc.want, u.Email)
		})
	}
}

func TestGetByEmail_NormalizesLookup(t *testing.T) {
	var queriedWith string
	repo := &mockUserRepo{
		GetByEmailFunc: func(ctx context.Context, email string) (*domain.User, error) {
			queriedWith = email
			return &domain.User{ID: "u1", Email: email}, nil
		},
	}
	uc := NewUserUseCase(repo)
	_, err := uc.GetByEmail(context.Background(), "  Alice@EXAMPLE.com ")
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", queriedWith)
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
	u := &domain.User{Email: "o@p.com", Role: "venue_owner"}
	require.NoError(t, uc.Create(context.Background(), u))
	require.NotNil(t, captured)
	assert.Equal(t, "venue_owner", captured.Role)
}

func TestCreate_RejectsInvalidRole(t *testing.T) {
	cases := []string{"partner", "root", "USER", "  user  ", "anonymous"}
	for _, role := range cases {
		t.Run(role, func(t *testing.T) {
			var repoCalls int
			repo := &mockUserRepo{
				CreateFunc: func(ctx context.Context, user *domain.User) error {
					repoCalls++
					return nil
				},
			}
			uc := NewUserUseCase(repo)
			u := &domain.User{Email: "x@y.com", Role: role}
			err := uc.Create(context.Background(), u)
			require.ErrorIs(t, err, domain.ErrInvalidRole)
			assert.Equal(t, 0, repoCalls, "repo.Create must not be called when role is invalid")
		})
	}
}

func TestCreate_AcceptsAllAllowedRoles(t *testing.T) {
	for role := range domain.AllowedRoles {
		t.Run(role, func(t *testing.T) {
			repo := &mockUserRepo{
				CreateFunc: func(ctx context.Context, user *domain.User) error { return nil },
			}
			uc := NewUserUseCase(repo)
			u := &domain.User{Email: "x@y.com", Role: role}
			require.NoError(t, uc.Create(context.Background(), u))
		})
	}
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
			return nil, domain.ErrNotFound
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.GetByID(context.Background(), "missing")
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, got)
}

func TestGetByEmail_NotFound(t *testing.T) {
	repo := &mockUserRepo{
		GetByEmailFunc: func(ctx context.Context, email string) (*domain.User, error) {
			return nil, domain.ErrNotFound
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.GetByEmail(context.Background(), "missing@example.com")
	require.ErrorIs(t, err, domain.ErrNotFound)
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

// TestUpdate_DeletedBetweenGetAndUpdate simulates the TOCTOU window where the
// row exists at GetByID time but is deleted before the UPDATE executes. The
// repository returns ErrNotFound from Update; the usecase must propagate it
// so the delivery layer returns NotFound instead of silent 200 OK.
func TestUpdate_DeletedBetweenGetAndUpdate(t *testing.T) {
	stored := &domain.User{ID: "u1", Email: "e@e.com", Name: "Old"}
	repo := &mockUserRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.User, error) {
			return stored, nil // row exists at read time
		},
		UpdateFunc: func(ctx context.Context, user *domain.User) error {
			return domain.ErrNotFound // deleted in the TOCTOU window
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.Update(context.Background(), "u1", stringPtr("New"), nil, nil, nil)
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, got)
}

func TestUpdate_UserNotFound(t *testing.T) {
	var updateCalls int
	repo := &mockUserRepo{
		GetByIDFunc: func(ctx context.Context, id string) (*domain.User, error) {
			return nil, domain.ErrNotFound
		},
		UpdateFunc: func(ctx context.Context, user *domain.User) error {
			updateCalls++
			return nil
		},
	}
	uc := NewUserUseCase(repo)
	got, err := uc.Update(context.Background(), "nope", stringPtr("x"), nil, nil, nil)
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, got)
	assert.Equal(t, 0, updateCalls, "repo.Update must not be called when GetByID returns ErrNotFound")
}
