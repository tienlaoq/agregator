package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Pool exposes the underlying pool for integration tests only.
func (r *Repo) Pool() *pgxpool.Pool { return r.pool }

func (r *Repo) EnsureSchema(ctx context.Context) error {
	const q = `
SELECT EXISTS (
  SELECT 1 FROM information_schema.tables
  WHERE table_schema = 'public' AND table_name = 'notifications'
)`
	var ok bool
	if err := r.pool.QueryRow(ctx, q).Scan(&ok); err != nil {
		return fmt.Errorf("verify notification schema: %w", err)
	}
	if !ok {
		return fmt.Errorf("notification schema is outdated: missing notifications table; apply migrations")
	}
	return nil
}

func scanNotification(row pgx.Row) (*domain.Notification, error) {
	var n domain.Notification
	var data *string
	var readAt *time.Time
	if err := row.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &data, &readAt, &n.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if data != nil {
		n.Data = *data
	}
	n.ReadAt = readAt
	return &n, nil
}

func (r *Repo) Create(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
	var data *string
	if n.Data != "" {
		data = &n.Data
	}
	out, err := scanNotification(r.pool.QueryRow(ctx, `
INSERT INTO notifications (user_id, type, title, body, data)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, type, title, body, data, read_at, created_at`,
		n.UserID, n.Type, n.Title, n.Body, data))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repo) List(ctx context.Context, userID uuid.UUID, limit, offset int32, unreadOnly bool) ([]domain.Notification, int32, error) {
	countQ := `SELECT COUNT(*) FROM notifications WHERE user_id = $1`
	listQ := `
SELECT id, user_id, type, title, body, data, read_at, created_at
FROM   notifications
WHERE  user_id = $1`
	if unreadOnly {
		countQ += ` AND read_at IS NULL`
		listQ += ` AND read_at IS NULL`
	}
	listQ += `
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

	var total int32
	if err := r.pool.QueryRow(ctx, countQ, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, listQ, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]domain.Notification, 0)
	for rows.Next() {
		var n domain.Notification
		var data *string
		var readAt *time.Time
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &data, &readAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		if data != nil {
			n.Data = *data
		}
		n.ReadAt = readAt
		out = append(out, n)
	}
	return out, total, rows.Err()
}

func (r *Repo) UnreadCount(ctx context.Context, userID uuid.UUID) (int32, error) {
	var c int32
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`,
		userID).Scan(&c)
	return c, err
}

func (r *Repo) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
UPDATE notifications SET read_at = now()
WHERE id = $1 AND user_id = $2 AND read_at IS NULL`, notificationID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
UPDATE notifications SET read_at = now()
WHERE user_id = $1 AND read_at IS NULL`, userID)
	return err
}

func (r *Repo) SaveDeviceToken(ctx context.Context, userID uuid.UUID, token, platform string) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO device_tokens (token, user_id, platform)
VALUES ($1, $2, $3)
ON CONFLICT (token) DO UPDATE
  SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = now()`,
		token, userID, platform)
	return err
}

func (r *Repo) DeleteDeviceToken(ctx context.Context, userID uuid.UUID, token string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM device_tokens WHERE token = $1 AND user_id = $2`, token, userID)
	return err
}

func (r *Repo) ListDeviceTokens(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT token FROM device_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repo) DeleteDeviceTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx,
		`DELETE FROM device_tokens WHERE token = ANY($1)`, tokens)
	return err
}
