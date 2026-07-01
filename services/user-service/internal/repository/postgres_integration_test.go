//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tienlao/agregator/services/user-service/internal/domain"
)

func TestCreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	u := seedUser(ctx, t, "master")

	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u.Email, got.Email)
	require.Equal(t, "master", got.Role)
	require.Nil(t, got.DeletedAt)
}

func TestGetByID_NotFound(t *testing.T) {
	_, err := newRepo().GetByID(context.Background(), uuid.NewString())
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetByEmail_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	u := seedUser(ctx, t, "user")

	// Stored email is lower-case; lookup must match regardless of case/space
	// (functional index idx_users_email_lower, migration 002).
	got, err := repo.GetByEmail(ctx, strings.ToLower(u.Email))
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)

	_, err = repo.GetByEmail(ctx, "missing-"+uuid.NewString()+"@example.com")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreate_DuplicateEmailRejected(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	u := seedUser(ctx, t, "user")

	dup := *u
	dup.ID = uuid.NewString()
	err := repo.Create(ctx, &dup)
	require.Error(t, err, "unique email constraint must reject a duplicate")
}

func TestGetBatch_PartialMissesOmitted(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	u1 := seedUser(ctx, t, "user")
	u2 := seedUser(ctx, t, "master")
	missing := uuid.NewString()

	out, err := repo.GetBatch(ctx, []string{u1.ID, u2.ID, missing})
	require.NoError(t, err)
	require.Len(t, out, 2, "missing ids must be silently omitted, not error")
	require.Contains(t, out, u1.ID)
	require.Contains(t, out, u2.ID)
	require.NotContains(t, out, missing)

	// Empty id list short-circuits to an empty map.
	empty, err := repo.GetBatch(ctx, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	u := seedUser(ctx, t, "user")

	u.Name = "Новое Имя"
	u.Bio = "обо мне"
	require.NoError(t, repo.Update(ctx, u))

	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "Новое Имя", got.Name)
	require.Equal(t, "обо мне", got.Bio)
}

func TestUpdate_NotFound(t *testing.T) {
	ctx := context.Background()
	u := &domain.User{ID: uuid.NewString(), Name: "X"}
	require.ErrorIs(t, newRepo().Update(ctx, u), domain.ErrNotFound)
}

func TestSoftDelete_AnonymisesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	u := seedUser(ctx, t, "user")

	require.NoError(t, repo.SoftDelete(ctx, u.ID))

	got, err := repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.DeletedAt, "deleted_at must be stamped")
	require.Equal(t, "Удалённый аккаунт", got.Name)
	require.Contains(t, got.Email, "@deleted.local", "email rewritten to tombstone")
	require.Equal(t, u.ID, strings.Split(strings.TrimPrefix(got.Email, "deleted+"), "@")[0])

	// Second delete affects 0 rows (deleted_at already set) → ErrNotFound.
	require.ErrorIs(t, repo.SoftDelete(ctx, u.ID), domain.ErrNotFound)
}

func TestSoftDelete_FreesEmailForReuse(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	u := seedUser(ctx, t, "user")
	originalEmail := u.Email

	require.NoError(t, repo.SoftDelete(ctx, u.ID))

	// The original address is freed (rewritten to a tombstone), so a brand-new
	// account can register it again without violating the unique index.
	reuse := &domain.User{
		ID: uuid.NewString(), Email: originalEmail, Name: "Reuse", Role: "user",
	}
	require.NoError(t, repo.Create(ctx, reuse))
}
