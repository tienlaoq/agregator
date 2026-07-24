package usecase

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	pkgerrors "github.com/tienlao/agregator/pkg/errors"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

const (
	maxTypeRunes  = 100
	maxTitleRunes = 200
	maxBodyRunes  = 2000
	maxDataBytes  = 8000
	maxTokenBytes = 4096
	defaultLimit  = 30
	maxLimit      = 100
	// pushTimeout bounds the detached mobile-push delivery for one Create.
	pushTimeout = 15 * time.Second
)

// Pusher delivers a notification to a set of device tokens via a push provider
// (FCM). It returns the tokens the provider reported as permanently gone so the
// usecase can prune them. Defined at the consumer site; a nil Pusher disables
// mobile push (the in-app bell still works).
type Pusher interface {
	Push(ctx context.Context, tokens []domain.DeviceToken, n *domain.Notification) (invalid []string, err error)
}

// NotificationUseCase orchestrates per-user notification reads and writes.
type NotificationUseCase struct {
	repo   domain.Repository
	pusher Pusher
	log    zerolog.Logger
	// pushWG tracks detached push goroutines so shutdown can drain them before
	// the Postgres pool is closed underneath them. See WaitPush.
	pushWG sync.WaitGroup
}

// New builds the usecase. pusher may be nil, which disables mobile push (the
// in-app bell still works). Pass zerolog.Nop() for log in tests.
func New(repo domain.Repository, pusher Pusher, log zerolog.Logger) *NotificationUseCase {
	return &NotificationUseCase{repo: repo, pusher: pusher, log: log}
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
	created, err := uc.repo.Create(ctx, n)
	if err != nil {
		return nil, err
	}
	// Mobile push is best-effort and must not add FCM latency to the caller, so
	// it runs detached from the request context (which is cancelled once Create
	// returns). Bounded by pushTimeout so a stuck provider can't leak goroutines.
	if uc.pusher != nil {
		uc.pushWG.Add(1)
		go func() {
			defer uc.pushWG.Done()
			uc.deliverPush(context.WithoutCancel(ctx), created)
		}()
	}
	return created, nil
}

// WaitPush blocks until all in-flight push goroutines finish or timeout elapses,
// returning true if they drained cleanly. Call it during shutdown (after gRPC
// GracefulStop, so no new pushes start) and before the DB pool is closed, so
// pushes don't hit a closed pool mid-query. Each push is already bounded by
// pushTimeout, so timeout should be ≥ that.
func (uc *NotificationUseCase) WaitPush(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		uc.pushWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// deliverPush fans a freshly-created notification out to the user's registered
// devices via the Pusher, then prunes any tokens the provider reported dead.
// Runs in its own goroutine; recovers so a push bug can never crash the service.
func (uc *NotificationUseCase) deliverPush(ctx context.Context, n *domain.Notification) {
	defer func() {
		if rec := recover(); rec != nil {
			uc.log.Error().Interface("panic", rec).Msg("push delivery panicked")
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()

	tokens, err := uc.repo.ListDeviceTokens(ctx, n.UserID)
	if err != nil {
		uc.log.Warn().Err(err).Str("user_id", n.UserID.String()).Msg("push: list device tokens failed")
		return
	}
	if len(tokens) == 0 {
		return
	}
	invalid, err := uc.pusher.Push(ctx, tokens, n)
	if err != nil {
		uc.log.Warn().Err(err).Str("user_id", n.UserID.String()).Msg("push: delivery error")
	}
	if len(invalid) > 0 {
		if delErr := uc.repo.DeleteDeviceTokens(ctx, invalid); delErr != nil {
			uc.log.Warn().Err(delErr).Msg("push: prune invalid tokens failed")
		}
	}
}

// RegisterDevice stores (or refreshes) a mobile push token for a user.
func (uc *NotificationUseCase) RegisterDevice(ctx context.Context, userID uuid.UUID, token, platform string) error {
	if userID == uuid.Nil {
		return pkgerrors.InvalidArgument("user_id обязателен")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return pkgerrors.InvalidArgument("token обязателен")
	}
	if len(token) > maxTokenBytes {
		return pkgerrors.InvalidArgument("token слишком длинный")
	}
	// platform is descriptive metadata; validate against the known set so typos
	// don't accumulate. Empty is allowed (unknown/unspecified).
	platform = strings.ToLower(strings.TrimSpace(platform))
	switch platform {
	case "", "ios", "android", "web":
	default:
		return pkgerrors.InvalidArgument("platform недопустим")
	}
	return uc.repo.SaveDeviceToken(ctx, userID, token, platform)
}

// UnregisterDevice removes a mobile push token for a user (logout / opt-out).
func (uc *NotificationUseCase) UnregisterDevice(ctx context.Context, userID uuid.UUID, token string) error {
	if userID == uuid.Nil {
		return pkgerrors.InvalidArgument("user_id обязателен")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return pkgerrors.InvalidArgument("token обязателен")
	}
	return uc.repo.DeleteDeviceToken(ctx, userID, token)
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
