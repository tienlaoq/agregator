package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

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

func (r *BookingRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM bookings WHERE id = $1`, id)
	return err
}

func (r *BookingRepo) Create(ctx context.Context, b *domain.Booking) error {
	pkgCol := packageServiceIDsForInsert(b.PackageServiceIDs)
	hallCol := bookingHallIDsForInsert(b.HallIDs)
	const q = `
		INSERT INTO bookings (user_id, venue_id, venue_name, service_id, date, time_from, time_to, guests, comment, status, total_price, package_service_ids, booking_hall_ids)
		VALUES ($1, $2, $3, NULLIF($4, '')::UUID, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, q,
		b.UserID, b.VenueID, b.VenueName, b.ServiceID, b.Date,
		b.TimeFrom, b.TimeTo, b.Guests, b.Comment,
		b.Status, b.TotalPrice, pkgCol, hallCol,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

func (r *BookingRepo) GetByID(ctx context.Context, id string) (*domain.Booking, error) {
	const q = `
		SELECT id, user_id, venue_id, COALESCE(venue_name, ''), COALESCE(service_id::TEXT, ''), date, time_from::TEXT, time_to::TEXT,
		       guests, COALESCE(comment, ''), status, total_price, COALESCE(payment_id::TEXT, ''),
		       created_at, updated_at, package_service_ids, booking_hall_ids
		FROM bookings WHERE id = $1`
	b := &domain.Booking{}
	var pkg *string
	var halls *string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&b.ID, &b.UserID, &b.VenueID, &b.VenueName, &b.ServiceID, &b.Date,
		&b.TimeFrom, &b.TimeTo, &b.Guests, &b.Comment,
		&b.Status, &b.TotalPrice, &b.PaymentID,
		&b.CreatedAt, &b.UpdatedAt, &pkg, &halls,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("booking not found")
	}
	if err != nil {
		return nil, err
	}
	mergePackageServiceIDs(b, pkg)
	mergeBookingHallIDs(b, halls)
	return b, nil
}

func (r *BookingRepo) ListByUser(ctx context.Context, userID, status string, offset, limit int) ([]*domain.Booking, int, error) {
	countQ := `SELECT COUNT(*) FROM bookings WHERE user_id = $1`
	dataQ := `
		SELECT id, user_id, venue_id, COALESCE(venue_name, ''), COALESCE(service_id::TEXT, ''), date, time_from::TEXT, time_to::TEXT,
		       guests, COALESCE(comment, ''), status, total_price, COALESCE(payment_id::TEXT, ''),
		       created_at, updated_at, package_service_ids, booking_hall_ids
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
		var pkg *string
		var halls *string
		if err := rows.Scan(
			&b.ID, &b.UserID, &b.VenueID, &b.VenueName, &b.ServiceID, &b.Date,
			&b.TimeFrom, &b.TimeTo, &b.Guests, &b.Comment,
			&b.Status, &b.TotalPrice, &b.PaymentID,
			&b.CreatedAt, &b.UpdatedAt, &pkg, &halls,
		); err != nil {
			return nil, 0, err
		}
		mergePackageServiceIDs(b, pkg)
		mergeBookingHallIDs(b, halls)
		bookings = append(bookings, b)
	}
	return bookings, total, rows.Err()
}

func (r *BookingRepo) ListByVenue(ctx context.Context, venueID, status, date string, offset, limit int) ([]*domain.Booking, int, error) {
	countQ := `SELECT COUNT(*) FROM bookings WHERE venue_id = $1`
	dataQ := `
		SELECT id, user_id, venue_id, COALESCE(venue_name, ''), COALESCE(service_id::TEXT, ''), date, time_from::TEXT, time_to::TEXT,
		       guests, COALESCE(comment, ''), status, total_price, COALESCE(payment_id::TEXT, ''),
		       created_at, updated_at, package_service_ids, booking_hall_ids
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
		var pkg *string
		var halls *string
		if err := rows.Scan(
			&b.ID, &b.UserID, &b.VenueID, &b.VenueName, &b.ServiceID, &b.Date,
			&b.TimeFrom, &b.TimeTo, &b.Guests, &b.Comment,
			&b.Status, &b.TotalPrice, &b.PaymentID,
			&b.CreatedAt, &b.UpdatedAt, &pkg, &halls,
		); err != nil {
			return nil, 0, err
		}
		mergePackageServiceIDs(b, pkg)
		mergeBookingHallIDs(b, halls)
		bookings = append(bookings, b)
	}
	return bookings, total, rows.Err()
}

func (r *BookingRepo) ListBookingStaffNotes(ctx context.Context, bookingID string) ([]domain.BookingStaffNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, booking_id::text, venue_id::text, author_user_id::text, body, created_at
		FROM booking_staff_notes WHERE booking_id = $1::uuid
		ORDER BY created_at ASC`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BookingStaffNote
	for rows.Next() {
		var n domain.BookingStaffNote
		if err := rows.Scan(&n.ID, &n.BookingID, &n.VenueID, &n.AuthorUserID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *BookingRepo) AddBookingStaffNote(ctx context.Context, n *domain.BookingStaffNote) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO booking_staff_notes (booking_id, venue_id, author_user_id, body)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
		RETURNING id::text, created_at`,
		n.BookingID, n.VenueID, n.AuthorUserID, n.Body,
	).Scan(&n.ID, &n.CreatedAt)
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
	const q = `SELECT EXISTS(
		SELECT 1 FROM bookings
		WHERE user_id = $1 AND venue_id = $2 AND status = 'completed' AND payment_id IS NOT NULL
	)`
	var exists bool
	err := r.pool.QueryRow(ctx, q, userID, venueID).Scan(&exists)
	return exists, err
}

func (r *BookingRepo) AutoCompleteVisitEnded(ctx context.Context, visitTimeZone string) ([]domain.BookingCompletedRef, error) {
	const q = `
		UPDATE bookings b
		SET status = 'completed', updated_at = now()
		WHERE b.status = 'confirmed'
		  AND b.payment_id IS NOT NULL
		  AND ((b.date + b.time_to) AT TIME ZONE $1) <= now()
		RETURNING b.id::text, b.user_id::text, b.venue_id::text`
	rows, err := r.pool.Query(ctx, q, visitTimeZone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.BookingCompletedRef
	for rows.Next() {
		var ref domain.BookingCompletedRef
		if err := rows.Scan(&ref.ID, &ref.UserID, &ref.VenueID); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func pgArgN(n int) string {
	return strconv.Itoa(n)
}

func packageServiceIDsForInsert(ids []string) any {
	if len(ids) <= 1 {
		return nil
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return nil
	}
	return string(b)
}

func mergePackageServiceIDs(b *domain.Booking, pkg *string) {
	b.PackageServiceIDs = nil
	if pkg == nil || strings.TrimSpace(*pkg) == "" {
		return
	}
	var parsed []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(*pkg)), &parsed); err != nil {
		return
	}
	for _, id := range parsed {
		id = strings.TrimSpace(id)
		if id != "" {
			b.PackageServiceIDs = append(b.PackageServiceIDs, id)
		}
	}
}

func bookingHallIDsForInsert(ids []string) any {
	if len(ids) == 0 {
		return nil
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return nil
	}
	return string(b)
}

func mergeBookingHallIDs(b *domain.Booking, raw *string) {
	b.HallIDs = nil
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return
	}
	var parsed []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(*raw)), &parsed); err != nil {
		return
	}
	for _, id := range parsed {
		id = strings.TrimSpace(id)
		if id != "" {
			b.HallIDs = append(b.HallIDs, id)
		}
	}
}
