package domain

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, n *Notification) (*Notification, error)
	List(ctx context.Context, userID uuid.UUID, limit, offset int32, unreadOnly bool) ([]Notification, int32, error)
	UnreadCount(ctx context.Context, userID uuid.UUID) (int32, error)
	// MarkRead marks a single notification read for the owning user. Returns
	// false when no matching unread row was found (wrong owner or already read);
	// callers treat that as a no-op, not an error (idempotent).
	MarkRead(ctx context.Context, userID, notificationID uuid.UUID) (bool, error)
	MarkAllRead(ctx context.Context, userID uuid.UUID) error

	// SaveDeviceToken upserts a mobile push token for a user (idempotent).
	// Re-registering a token already owned by another user reassigns it.
	SaveDeviceToken(ctx context.Context, userID uuid.UUID, token, platform string) error
	// DeleteDeviceToken removes one token for a user (logout / opt-out).
	DeleteDeviceToken(ctx context.Context, userID uuid.UUID, token string) error
	// ListDeviceTokens returns every push token registered for a user.
	ListDeviceTokens(ctx context.Context, userID uuid.UUID) ([]string, error)
	// DeleteDeviceTokens prunes tokens FCM reported as invalid/unregistered.
	DeleteDeviceTokens(ctx context.Context, tokens []string) error
}
