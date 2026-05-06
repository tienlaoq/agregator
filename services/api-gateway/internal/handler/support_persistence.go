package handler

import (
	"context"

	"github.com/google/uuid"
	"github.com/tienlao/agregator/services/api-gateway/internal/supportstore"
)

// SupportTicketsPersistence — хранение очереди обращений (Postgres). Интерфейс для подстановки стабов в тестах.
type SupportTicketsPersistence interface {
	Insert(ctx context.Context, p supportstore.InsertParams) error
	SetNotifyStatus(ctx context.Context, requestID uuid.UUID, status string) error
	List(ctx context.Context, limit, offset int) ([]supportstore.Row, int64, error)
	GetByRequestID(ctx context.Context, id uuid.UUID) (*supportstore.Row, error)
	GetByTicketNumber(ctx context.Context, ticketNumber string) (*supportstore.Row, error)
	MarkReplied(ctx context.Context, id uuid.UUID, moderatorID string) error
}

var _ SupportTicketsPersistence = (*supportstore.Store)(nil)
