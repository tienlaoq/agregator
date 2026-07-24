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
	var haveNotifications, haveDeviceTokens bool
	if err := r.pool.QueryRow(ctx, qEnsureSchema).Scan(&haveNotifications, &haveDeviceTokens); err != nil {
		return fmt.Errorf("verify notification schema: %w", err)
	}
	if !haveNotifications || !haveDeviceTokens {
		return fmt.Errorf("notification schema is outdated: missing tables (notifications=%t device_tokens=%t); apply migrations", haveNotifications, haveDeviceTokens)
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
	out, err := scanNotification(r.pool.QueryRow(ctx, qCreate,
		n.UserID, n.Type, n.Title, n.Body, data))
	if err != nil {
		return nil, err
	}
	// scanNotification maps ErrNoRows to (nil, nil) for the lookup paths, but an
	// INSERT ... RETURNING always yields a row — a nil here means that contract
	// broke, so fail loudly instead of returning a nil the usecase derefs.
	if out == nil {
		return nil, fmt.Errorf("create notification: insert returned no row")
	}
	return out, nil
}

func (r *Repo) List(ctx context.Context, userID uuid.UUID, limit, offset int32, unreadOnly bool) ([]domain.Notification, int32, error) {
	countQ, listQ := qCountAll, qListAll
	if unreadOnly {
		countQ, listQ = qCountUnread, qListUnread
	}

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
	err := r.pool.QueryRow(ctx, qUnreadCount, userID).Scan(&c)
	return c, err
}

func (r *Repo) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, qMarkRead, notificationID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, qMarkAllRead, userID)
	return err
}

func (r *Repo) SaveDeviceToken(ctx context.Context, userID uuid.UUID, token, platform string) error {
	_, err := r.pool.Exec(ctx, qSaveDeviceToken, token, userID, platform)
	return err
}

func (r *Repo) DeleteDeviceToken(ctx context.Context, userID uuid.UUID, token string) error {
	_, err := r.pool.Exec(ctx, qDeleteDeviceToken, token, userID)
	return err
}

func (r *Repo) ListDeviceTokens(ctx context.Context, userID uuid.UUID) ([]domain.DeviceToken, error) {
	rows, err := r.pool.Query(ctx, qListDeviceTokens, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DeviceToken
	for rows.Next() {
		var t domain.DeviceToken
		if err := rows.Scan(&t.Token, &t.Platform); err != nil {
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
	_, err := r.pool.Exec(ctx, qDeleteDeviceTokens, tokens)
	return err
}
