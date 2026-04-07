package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/user-service/internal/domain"
)

type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

func (r *PostgresUserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, phone, name, role, avatar_url, bio, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		user.ID, user.Email, user.Phone, user.Name, user.Role,
		user.AvatarURL, user.Bio, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "email") {
				return domain.ErrEmailExists
			}
			if pgErr.Code == "23514" && strings.Contains(pgErr.ConstraintName, "role") {
				return domain.ErrInvalidRole
			}
		}
		return err
	}
	return nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, phone, name, role, avatar_url, bio, created_at, updated_at
		 FROM users WHERE id = $1`, id))
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, phone, name, role, avatar_url, bio, created_at, updated_at
		 FROM users WHERE email = $1`, email))
}

func (r *PostgresUserRepository) Update(ctx context.Context, user *domain.User) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET name = $2, phone = $3, avatar_url = $4, bio = $5, updated_at = $6
		 WHERE id = $1`,
		user.ID, user.Name, user.Phone, user.AvatarURL, user.Bio, user.UpdatedAt,
	)
	return err
}

func (r *PostgresUserRepository) scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(
		&u.ID, &u.Email, &u.Phone, &u.Name, &u.Role,
		&u.AvatarURL, &u.Bio, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
