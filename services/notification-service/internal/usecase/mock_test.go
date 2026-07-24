package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/tienlao/agregator/services/notification-service/internal/domain"
)

// newTestUC builds a usecase with push disabled (nil Pusher) and a no-op logger
// — the default for tests that don't exercise mobile push.
func newTestUC(repo domain.Repository) *NotificationUseCase {
	return New(repo, nil, zerolog.Nop())
}

// mockRepo is a hand-written func-field mock of domain.Repository. Unset fields
// return zero values, so each test wires only the methods it exercises.
type mockRepo struct {
	CreateFunc             func(ctx context.Context, n *domain.Notification) (*domain.Notification, error)
	ListFunc               func(ctx context.Context, userID uuid.UUID, limit, offset int32, unreadOnly bool) ([]domain.Notification, int32, error)
	UnreadCountFunc        func(ctx context.Context, userID uuid.UUID) (int32, error)
	MarkReadFunc           func(ctx context.Context, userID, notificationID uuid.UUID) (bool, error)
	MarkAllReadFunc        func(ctx context.Context, userID uuid.UUID) error
	SaveDeviceTokenFunc    func(ctx context.Context, userID uuid.UUID, token, platform string) error
	DeleteDeviceTokenFunc  func(ctx context.Context, userID uuid.UUID, token string) error
	ListDeviceTokensFunc   func(ctx context.Context, userID uuid.UUID) ([]string, error)
	DeleteDeviceTokensFunc func(ctx context.Context, tokens []string) error
}

func (m *mockRepo) Create(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, n)
	}
	return n, nil
}

func (m *mockRepo) List(ctx context.Context, userID uuid.UUID, limit, offset int32, unreadOnly bool) ([]domain.Notification, int32, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, userID, limit, offset, unreadOnly)
	}
	return nil, 0, nil
}

func (m *mockRepo) UnreadCount(ctx context.Context, userID uuid.UUID) (int32, error) {
	if m.UnreadCountFunc != nil {
		return m.UnreadCountFunc(ctx, userID)
	}
	return 0, nil
}

func (m *mockRepo) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) (bool, error) {
	if m.MarkReadFunc != nil {
		return m.MarkReadFunc(ctx, userID, notificationID)
	}
	return false, nil
}

func (m *mockRepo) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if m.MarkAllReadFunc != nil {
		return m.MarkAllReadFunc(ctx, userID)
	}
	return nil
}

func (m *mockRepo) SaveDeviceToken(ctx context.Context, userID uuid.UUID, token, platform string) error {
	if m.SaveDeviceTokenFunc != nil {
		return m.SaveDeviceTokenFunc(ctx, userID, token, platform)
	}
	return nil
}

func (m *mockRepo) DeleteDeviceToken(ctx context.Context, userID uuid.UUID, token string) error {
	if m.DeleteDeviceTokenFunc != nil {
		return m.DeleteDeviceTokenFunc(ctx, userID, token)
	}
	return nil
}

func (m *mockRepo) ListDeviceTokens(ctx context.Context, userID uuid.UUID) ([]string, error) {
	if m.ListDeviceTokensFunc != nil {
		return m.ListDeviceTokensFunc(ctx, userID)
	}
	return nil, nil
}

func (m *mockRepo) DeleteDeviceTokens(ctx context.Context, tokens []string) error {
	if m.DeleteDeviceTokensFunc != nil {
		return m.DeleteDeviceTokensFunc(ctx, tokens)
	}
	return nil
}
