package domain

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	EnsureThread(ctx context.Context, kind string, refID uuid.UUID, participantUserIDs []string) (*Thread, error)
	GetThreadByID(ctx context.Context, threadID uuid.UUID) (*Thread, error)
	ListThreadsForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]Thread, int32, error)
	ListMessages(ctx context.Context, threadID uuid.UUID, limit, offset int32) ([]Message, int32, error)
	InsertMessage(ctx context.Context, threadID, authorUserID uuid.UUID, text, clientMsgID string) (*Message, error)
	MarkRead(ctx context.Context, threadID, userID uuid.UUID) error
}

