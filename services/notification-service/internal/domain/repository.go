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
}
