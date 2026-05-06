package supportstore

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("support ticket not found")

type InsertParams struct {
	RequestID    uuid.UUID
	TicketNumber string
	Topic        string
	Message      string
	UserEmail    string
	UserID       string
	Role         string
	BookingID    string
	PaymentID    string
	SourcePage   string
}

type Row struct {
	RequestID    uuid.UUID
	TicketNumber string
	Topic        string
	Message      string
	UserEmail    string
	UserID       string
	Role         string
	BookingID    string
	PaymentID    string
	SourcePage   string
	CreatedAt    time.Time
	RepliedAt    *time.Time
	RepliedBy    string
	NotifyStatus string
}

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	if pool == nil {
		return nil
	}
	return &Store{pool: pool}
}

func (s *Store) Insert(ctx context.Context, p InsertParams) error {
	const q = `
INSERT INTO support_tickets (
  request_id, ticket_number, topic, message,
  user_email, user_id, role, booking_id, payment_id, source_page, notify_status
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`
	_, err := s.pool.Exec(ctx, q,
		p.RequestID, p.TicketNumber, p.Topic, p.Message,
		p.UserEmail, p.UserID, p.Role, p.BookingID, p.PaymentID, p.SourcePage,
		NotifyPending,
	)
	return err
}

// SetNotifyStatus обновляет результат доставки уведомления модератору (SMTP/webhook).
func (s *Store) SetNotifyStatus(ctx context.Context, requestID uuid.UUID, status string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE support_tickets SET notify_status = $2 WHERE request_id = $1`,
		requestID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) List(ctx context.Context, limit, offset int) ([]Row, int64, error) {
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM support_tickets`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT request_id, ticket_number, topic, message, user_email, user_id, role,
       booking_id, payment_id, source_page, created_at, replied_at, replied_by, notify_status
FROM support_tickets
ORDER BY created_at DESC
LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(
			&r.RequestID, &r.TicketNumber, &r.Topic, &r.Message, &r.UserEmail,
			&r.UserID, &r.Role, &r.BookingID, &r.PaymentID, &r.SourcePage,
			&r.CreatedAt, &r.RepliedAt, &r.RepliedBy, &r.NotifyStatus,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) GetByRequestID(ctx context.Context, id uuid.UUID) (*Row, error) {
	const q = `
SELECT request_id, ticket_number, topic, message, user_email, user_id, role,
       booking_id, payment_id, source_page, created_at, replied_at, replied_by, notify_status
FROM support_tickets WHERE request_id = $1`
	var r Row
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&r.RequestID, &r.TicketNumber, &r.Topic, &r.Message, &r.UserEmail,
		&r.UserID, &r.Role, &r.BookingID, &r.PaymentID, &r.SourcePage,
		&r.CreatedAt, &r.RepliedAt, &r.RepliedBy, &r.NotifyStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetByTicketNumber(ctx context.Context, ticketNumber string) (*Row, error) {
	tn := strings.TrimSpace(strings.ToUpper(ticketNumber))
	const q = `
SELECT request_id, ticket_number, topic, message, user_email, user_id, role,
       booking_id, payment_id, source_page, created_at, replied_at, replied_by, notify_status
FROM support_tickets WHERE UPPER(TRIM(ticket_number)) = $1`
	var r Row
	err := s.pool.QueryRow(ctx, q, tn).Scan(
		&r.RequestID, &r.TicketNumber, &r.Topic, &r.Message, &r.UserEmail,
		&r.UserID, &r.Role, &r.BookingID, &r.PaymentID, &r.SourcePage,
		&r.CreatedAt, &r.RepliedAt, &r.RepliedBy, &r.NotifyStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) MarkReplied(ctx context.Context, id uuid.UUID, moderatorID string) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE support_tickets SET replied_at = now(), replied_by = $2 WHERE request_id = $1`,
		id, strings.TrimSpace(moderatorID))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
