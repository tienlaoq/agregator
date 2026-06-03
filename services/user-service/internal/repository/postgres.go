package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/user-service/internal/domain"
)

type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

// Create inserts a user row. Postgres-level errors are returned as-is and
// mapped to gRPC status codes by grpcutil.PgErrorUnaryInterceptor:
//   - 23505 unique_violation → AlreadyExists (generic, no Detail leak)
//
// CHECK violations on the role column should never happen in practice: the
// usecase layer validates role against domain.AllowedRoles before reaching the
// repository, so any 23514 here means the migration drifted from the code.
func (r *PostgresUserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, phone, name, role, avatar_url, bio, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		user.ID, user.Email, user.Phone, user.Name, user.Role,
		user.AvatarURL, user.Bio, user.CreatedAt, user.UpdatedAt,
	)
	return err
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, phone, name, role, avatar_url, bio, created_at, updated_at, deleted_at
		 FROM users WHERE id = $1`, id))
}

func (r *PostgresUserRepository) GetBatch(ctx context.Context, ids []string) (map[string]*domain.User, error) {
	if len(ids) == 0 {
		return map[string]*domain.User{}, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, email, phone, name, role, avatar_url, bio, created_at, updated_at, deleted_at
		 FROM users WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]*domain.User, len(ids))
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Phone, &u.Name, &u.Role,
			&u.AvatarURL, &u.Bio, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
		); err != nil {
			return nil, err
		}
		out[u.ID] = &u
	}
	return out, rows.Err()
}

// GetByEmail looks up a user by email address.
//
// The caller MUST normalise the email to lower(trim(...)) before calling this
// method — the usecase layer does this in normalizeEmail. The WHERE clause
// matches on lower(trim(email)) so the lookup is case- and whitespace-insensitive,
// and the predicate is index-backed (idx_users_email_lower). Passing a
// pre-normalised value on the right-hand side avoids the double lower(trim($1))
// call that would otherwise be needed.
func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, phone, name, role, avatar_url, bio, created_at, updated_at, deleted_at
		 FROM users WHERE lower(trim(email)) = $1`, email))
}

// SoftDelete anonymises and deactivates the user in one UPDATE. The email is
// rewritten to a per-id tombstone so the original address is freed for a future
// re-registration while the functional unique index on lower(trim(email)) stays
// satisfied. WHERE deleted_at IS NULL makes the call idempotent: a repeat on an
// already-deleted row affects 0 rows and surfaces ErrNotFound.
func (r *PostgresUserRepository) SoftDelete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users
		 SET email      = 'deleted+' || id::text || '@deleted.local',
		     name       = 'Удалённый аккаунт',
		     phone      = NULL,
		     avatar_url = NULL,
		     bio        = NULL,
		     deleted_at = now(),
		     updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresUserRepository) Update(ctx context.Context, user *domain.User) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET name = $2, phone = $3, avatar_url = $4, bio = $5, updated_at = $6
		 WHERE id = $1`,
		user.ID, user.Name, user.Phone, user.AvatarURL, user.Bio, user.UpdatedAt,
	)
	if err != nil {
		return err
	}
	// RowsAffected == 0 means the row was deleted between the preceding
	// GetByID and this UPDATE (TOCTOU window). Surface the same sentinel
	// the read path uses so the delivery layer produces a consistent
	// NotFound response instead of a silent 200 OK.
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *PostgresUserRepository) scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(
		&u.ID, &u.Email, &u.Phone, &u.Name, &u.Role,
		&u.AvatarURL, &u.Bio, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Translate the pgx sentinel into the domain sentinel so the usecase
		// and delivery layers never reach for pgx-specific error types.
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
