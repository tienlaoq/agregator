package repository

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

type BookingRepo struct {
	pool *pgxpool.Pool
}

func NewBookingRepo(pool *pgxpool.Pool) *BookingRepo {
	return &BookingRepo{pool: pool}
}

func (r *BookingRepo) Create(ctx context.Context, b *domain.Booking) error {
	const q = `
		INSERT INTO bookings (user_id, venue_id, venue_name, service_id, date, time_from, time_to, guests, comment, status, total_price)
		VALUES ($1, $2, $3, NULLIF($4, '')::UUID, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, q,
		b.UserID, b.VenueID, b.VenueName, b.ServiceID, b.Date,
		b.TimeFrom, b.TimeTo, b.Guests, b.Comment,
		b.Status, b.TotalPrice,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

func (r *BookingRepo) GetByID(ctx context.Context, id string) (*domain.Booking, error) {
	const q = `
		SELECT id, user_id, venue_id, COALESCE(venue_name, ''), COALESCE(service_id::TEXT, ''), date, time_from::TEXT, time_to::TEXT,
		       guests, COALESCE(comment, ''), status, total_price, COALESCE(payment_id::TEXT, ''),
		       created_at, updated_at
		FROM bookings WHERE id = $1`
	b := &domain.Booking{}
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&b.ID, &b.UserID, &b.VenueID, &b.VenueName, &b.ServiceID, &b.Date,
		&b.TimeFrom, &b.TimeTo, &b.Guests, &b.Comment,
		&b.Status, &b.TotalPrice, &b.PaymentID,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("booking not found")
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *BookingRepo) ListByUser(ctx context.Context, userID, status string, offset, limit int) ([]*domain.Booking, int, error) {
	countQ := `SELECT COUNT(*) FROM bookings WHERE user_id = $1`
	dataQ := `
		SELECT id, user_id, venue_id, COALESCE(venue_name, ''), COALESCE(service_id::TEXT, ''), date, time_from::TEXT, time_to::TEXT,
		       guests, COALESCE(comment, ''), status, total_price, COALESCE(payment_id::TEXT, ''),
		       created_at, updated_at
		FROM bookings WHERE user_id = $1`
	args := []any{userID}

	if status != "" {
		countQ += ` AND status = $2`
		dataQ += ` AND status = $2`
		args = append(args, status)
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQ += ` ORDER BY created_at DESC OFFSET $` + pgArgN(len(args)+1) + ` LIMIT $` + pgArgN(len(args)+2)
	args = append(args, offset, limit)

	rows, err := r.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var bookings []*domain.Booking
	for rows.Next() {
		b := &domain.Booking{}
		if err := rows.Scan(
			&b.ID, &b.UserID, &b.VenueID, &b.VenueName, &b.ServiceID, &b.Date,
			&b.TimeFrom, &b.TimeTo, &b.Guests, &b.Comment,
			&b.Status, &b.TotalPrice, &b.PaymentID,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		bookings = append(bookings, b)
	}
	return bookings, total, rows.Err()
}

func (r *BookingRepo) ListByVenue(ctx context.Context, venueID, status, date string, offset, limit int) ([]*domain.Booking, int, error) {
	countQ := `SELECT COUNT(*) FROM bookings WHERE venue_id = $1`
	dataQ := `
		SELECT id, user_id, venue_id, COALESCE(venue_name, ''), COALESCE(service_id::TEXT, ''), date, time_from::TEXT, time_to::TEXT,
		       guests, COALESCE(comment, ''), status, total_price, COALESCE(payment_id::TEXT, ''),
		       created_at, updated_at
		FROM bookings WHERE venue_id = $1`
	args := []any{venueID}
	n := 1

	if status != "" {
		n++
		countQ += ` AND status = $` + pgArgN(n)
		dataQ += ` AND status = $` + pgArgN(n)
		args = append(args, status)
	}
	if date != "" {
		n++
		countQ += ` AND date = $` + pgArgN(n)
		dataQ += ` AND date = $` + pgArgN(n)
		args = append(args, date)
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dataQ += ` ORDER BY date, time_from OFFSET $` + pgArgN(n+1) + ` LIMIT $` + pgArgN(n+2)
	args = append(args, offset, limit)

	rows, err := r.pool.Query(ctx, dataQ, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var bookings []*domain.Booking
	for rows.Next() {
		b := &domain.Booking{}
		if err := rows.Scan(
			&b.ID, &b.UserID, &b.VenueID, &b.VenueName, &b.ServiceID, &b.Date,
			&b.TimeFrom, &b.TimeTo, &b.Guests, &b.Comment,
			&b.Status, &b.TotalPrice, &b.PaymentID,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		bookings = append(bookings, b)
	}
	return bookings, total, rows.Err()
}

func (r *BookingRepo) UpdateStatus(ctx context.Context, id, status string) error {
	const q = `UPDATE bookings SET status = $2, updated_at = now() WHERE id = $1`
	ct, err := r.pool.Exec(ctx, q, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pkgerr.NotFound("booking not found")
	}
	return nil
}

func (r *BookingRepo) SetPaymentID(ctx context.Context, bookingID, paymentID string) error {
	const q = `UPDATE bookings SET payment_id = $2, updated_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, bookingID, paymentID)
	return err
}

func (r *BookingRepo) HasCompleted(ctx context.Context, userID, venueID string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM bookings WHERE user_id = $1 AND venue_id = $2 AND status = 'completed')`
	var exists bool
	err := r.pool.QueryRow(ctx, q, userID, venueID).Scan(&exists)
	return exists, err
}

func pgArgN(n int) string {
	return strconv.Itoa(n)
}
