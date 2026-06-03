package usecase

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

const (
	maxTypeRunes  = 100
	maxTitleRunes = 200
	maxBodyRunes  = 2000
	maxDataBytes  = 8000
	defaultLimit  = 30
	maxLimit      = 100
)

// NotificationUseCase orchestrates per-user notification reads and writes.
type NotificationUseCase struct {
	repo domain.Repository
}

func New(repo domain.Repository) *NotificationUseCase {
	return &NotificationUseCase{repo: repo}
}

func (uc *NotificationUseCase) Create(ctx context.Context, userID uuid.UUID, typ, title, body, data string) (*domain.Notification, error) {
	if userID == uuid.Nil {
		return nil, pkgerrors.InvalidArgument("user_id обязателен")
	}
	typ = strings.TrimSpace(typ)
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	data = strings.TrimSpace(data)
	if typ == "" {
		return nil, pkgerrors.InvalidArgument("type обязателен")
	}
	if utf8.RuneCountInString(typ) > maxTypeRunes {
		return nil, pkgerrors.InvalidArgument("type слишком длинный")
	}
	if title == "" {
		return nil, pkgerrors.InvalidArgument("title обязателен")
	}
	if utf8.RuneCountInString(title) > maxTitleRunes {
		return nil, pkgerrors.InvalidArgument("title слишком длинный")
	}
	if utf8.RuneCountInString(body) > maxBodyRunes {
		return nil, pkgerrors.InvalidArgument("body слишком длинный")
	}
	if len(data) > maxDataBytes {
		return nil, pkgerrors.InvalidArgument("data слишком большой")
	}
	n := &domain.Notification{
		UserID: userID,
		Type:   typ,
		Title:  title,
		Body:   body,
		Data:   data,
	}
	return uc.repo.Create(ctx, n)
}

// List returns the page of notifications plus the total matching the filter and
// the user's current unread count (for the badge), in one call.
func (uc *NotificationUseCase) List(ctx context.Context, userID uuid.UUID, limit, offset int32, unreadOnly bool) ([]domain.Notification, int32, int32, error) {
	if userID == uuid.Nil {
		return nil, 0, 0, pkgerrors.InvalidArgument("user_id обязателен")
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	list, total, err := uc.repo.List(ctx, userID, limit, offset, unreadOnly)
	if err != nil {
		return nil, 0, 0, err
	}
	unread, err := uc.repo.UnreadCount(ctx, userID)
	if err != nil {
		return nil, 0, 0, err
	}
	return list, total, unread, nil
}

func (uc *NotificationUseCase) UnreadCount(ctx context.Context, userID uuid.UUID) (int32, error) {
	if userID == uuid.Nil {
		return 0, pkgerrors.InvalidArgument("user_id обязателен")
	}
	return uc.repo.UnreadCount(ctx, userID)
}

// MarkRead marks one notification read (no-op when already read or not owned by
// the user) and returns the refreshed unread count.
func (uc *NotificationUseCase) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) (int32, error) {
	if userID == uuid.Nil || notificationID == uuid.Nil {
		return 0, pkgerrors.InvalidArgument("user_id и notification_id обязательны")
	}
	if _, err := uc.repo.MarkRead(ctx, userID, notificationID); err != nil {
		return 0, err
	}
	return uc.repo.UnreadCount(ctx, userID)
}

func (uc *NotificationUseCase) MarkAllRead(ctx context.Context, userID uuid.UUID) (int32, error) {
	if userID == uuid.Nil {
		return 0, pkgerrors.InvalidArgument("user_id обязателен")
	}
	if err := uc.repo.MarkAllRead(ctx, userID); err != nil {
		return 0, err
	}
	return uc.repo.UnreadCount(ctx, userID)
}
