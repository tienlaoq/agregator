package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/booking-service/internal/domain"
)

type BookingRepo struct {
	pool          *pgxpool.Pool
	cursorHMACKey []byte
}

// NewBookingRepo создаёт репозиторий. cursorHMACKey используется для подписи
// keyset-курсоров; передавайте непустое значение из config.CursorHMACKey.
// Пустая строка допускается (dev/test), но курсоры тогда подписываются
// fallback-ключом — подделка возможна.
func NewBookingRepo(pool *pgxpool.Pool, cursorHMACKey string) *BookingRepo {
	key := []byte(cursorHMACKey)
	if len(key) == 0 {
		// Небезопасный fallback для dev-окружения. В production задайте CURSOR_HMAC_KEY.
		key = []byte("dev-insecure-cursor-key")
	}
	return &BookingRepo{pool: pool, cursorHMACKey: key}
}

func (r *BookingRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM bookings WHERE id = $1`, id)
	return err
}

func (r *BookingRepo) Create(ctx context.Context, b *domain.Booking) error {
	const q = `
		INSERT INTO bookings (user_id, venue_id, venue_name, service_id, date, time_from, time_to, guests, comment, status, total_price, package_service_ids, booking_hall_ids)
		VALUES ($1, $2, $3, NULLIF($4, '')::UUID, $5, $6, $7, $8, $9, $10, $11, $12::uuid[], $13::uuid[])
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, q,
		b.UserID, b.VenueID, b.VenueName, b.ServiceID, b.Date,
		b.TimeFrom, b.TimeTo, b.Guests, b.Comment,
		b.Status, b.TotalPrice,
		nullableUUIDs(b.PackageServiceIDs),
		nullableUUIDs(b.HallIDs),
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

// insertOutboxTx пишет одно событие в outbox в рамках переданной транзакции.
// payload хранится как JSONB (передаётся как text, PG валидирует JSON).
func insertOutboxTx(ctx context.Context, tx pgx.Tx, ev domain.OutboxEvent) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO outbox_events (subject, payload) VALUES ($1, $2::jsonb)`,
		ev.Subject, string(ev.Payload))
	return err
}

// SetPaymentAndStatus атомарно обновляет payment_id, payment_url и status и в той же
// транзакции пишет событие booking.created в outbox. Используется в CreateBooking.
func (r *BookingRepo) SetPaymentAndStatus(ctx context.Context, bookingID, paymentID, paymentURL, status string, ev domain.OutboxEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	const q = `UPDATE bookings SET payment_id = $2, payment_url = $3, status = $4, updated_at = now() WHERE id = $1`
	ct, err := tx.Exec(ctx, q, bookingID, paymentID, paymentURL, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pkgerr.NotFound("booking not found")
	}
	if err := insertOutboxTx(ctx, tx, ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CancelWithEvent переводит бронь в status='cancelled' и пишет booking.cancelled
// в outbox в одной транзакции. Гарантии отмены проверяются в usecase.
func (r *BookingRepo) CancelWithEvent(ctx context.Context, id string, ev domain.OutboxEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	const q = `UPDATE bookings SET status = $2, updated_at = now() WHERE id = $1`
	ct, err := tx.Exec(ctx, q, id, string(domain.StatusCancelled))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pkgerr.NotFound("booking not found")
	}
	if err := insertOutboxTx(ctx, tx, ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FetchUnsentOutbox возвращает неотправленные события в порядке id (FIFO).
// payload::text — чтобы получить JSON-байты в string без двусмысленности pgx jsonb→[]byte.
func (r *BookingRepo) FetchUnsentOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	const q = `SELECT id, subject, payload::text FROM outbox_events WHERE sent_at IS NULL ORDER BY id LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.OutboxEvent
	for rows.Next() {
		var (
			ev      domain.OutboxEvent
			payload string
		)
		if err := rows.Scan(&ev.ID, &ev.Subject, &payload); err != nil {
			return nil, err
		}
		ev.Payload = []byte(payload)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// MarkOutboxSent проставляет sent_at = now() для отправленных событий. Пустой срез — no-op.
func (r *BookingRepo) MarkOutboxSent(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `UPDATE outbox_events SET sent_at = now() WHERE id = ANY($1::bigint[])`, ids)
	return err
}

// selectCols — общие колонки для всех SELECT-запросов на bookings.
// Порядок должен совпадать с scanDest.
// time_from/time_to передаются как TEXT ("HH:MM:SS") — scanDest направляет их
// в domain.TimeOfDay.Scan, который обрезает секунды и валидирует формат.
const selectCols = `
	id, user_id, venue_id, COALESCE(venue_name, ''), COALESCE(service_id::TEXT, ''), date, time_from::TEXT, time_to::TEXT,
	guests, COALESCE(comment, ''), status, total_price, COALESCE(payment_id::TEXT, ''),
	created_at, updated_at,
	COALESCE(package_service_ids::text[], '{}'::text[]),
	COALESCE(booking_hall_ids::text[], '{}'::text[]),
	COALESCE(payment_url, '')`

// scanDest возвращает срез scan-указателей для одной строки bookings.
// extra принимает дополнительные назначения (например &total для COUNT(*) OVER()).
// Порядок должен совпадать с selectCols.
//
// Единая точка правды: любое изменение схемы (добавление колонки в selectCols)
// требует правки только здесь.
//
// b.TimeFrom и b.TimeFrom реализуют sql.Scanner — pgx передаёт строку "HH:MM:SS"
// и TimeOfDay.Scan обрезает секунды, валидирует диапазон.
func scanDest(b *domain.Booking, pkg, halls *[]string, extra ...any) []any {
	dest := []any{
		&b.ID, &b.UserID, &b.VenueID, &b.VenueName, &b.ServiceID, &b.Date,
		&b.TimeFrom, &b.TimeTo, &b.Guests, &b.Comment,
		&b.Status, &b.TotalPrice, &b.PaymentID,
		&b.CreatedAt, &b.UpdatedAt, pkg, halls,
		&b.PaymentURL,
	}
	return append(dest, extra...)
}

// scanBooking сканирует одну строку (pgx.Row) в *domain.Booking.
func scanBooking(row pgx.Row) (*domain.Booking, error) {
	b := &domain.Booking{}
	var pkg, halls []string
	if err := row.Scan(scanDest(b, &pkg, &halls)...); err != nil {
		return nil, err
	}
	b.PackageServiceIDs = nonEmpty(pkg)
	b.HallIDs = nonEmpty(halls)
	return b, nil
}

// scanBookingRow сканирует текущую строку из pgx.Rows в *domain.Booking.
func scanBookingRow(rows pgx.Rows) (*domain.Booking, error) {
	b := &domain.Booking{}
	var pkg, halls []string
	if err := rows.Scan(scanDest(b, &pkg, &halls)...); err != nil {
		return nil, err
	}
	b.PackageServiceIDs = nonEmpty(pkg)
	b.HallIDs = nonEmpty(halls)
	return b, nil
}

// scanBookingFromRows сканирует строку из pgx.Rows вместе с булевым флагом confirmed.
// Используется в ConfirmPayment CTE чтобы различить UPDATE-ветку от SELECT-ветки
// без второго round-trip.
func scanBookingFromRows(rows pgx.Rows, confirmed *bool) (*domain.Booking, error) {
	b := &domain.Booking{}
	var pkg, halls []string
	if err := rows.Scan(scanDest(b, &pkg, &halls, confirmed)...); err != nil {
		return nil, err
	}
	b.PackageServiceIDs = nonEmpty(pkg)
	b.HallIDs = nonEmpty(halls)
	return b, nil
}

// scanBookingWithTotal сканирует строку из pgx.Rows вместе с дополнительным
// столбцом total (COUNT(*) OVER()), не дублируя список полей.
func scanBookingWithTotal(rows pgx.Rows, total *int) (*domain.Booking, error) {
	b := &domain.Booking{}
	var pkg, halls []string
	if err := rows.Scan(scanDest(b, &pkg, &halls, total)...); err != nil {
		return nil, err
	}
	b.PackageServiceIDs = nonEmpty(pkg)
	b.HallIDs = nonEmpty(halls)
	return b, nil
}

// scanBookingWithTotalAndNotes добавляет к total ещё и staff_notes_count —
// используется CRM-реестром (ListByVenue), который подтягивает счётчик заметок.
// Порядок extra-колонок: total, затем staff_notes_count (см. SELECT в ListByVenue).
func scanBookingWithTotalAndNotes(rows pgx.Rows, total *int) (*domain.Booking, error) {
	b := &domain.Booking{}
	var pkg, halls []string
	if err := rows.Scan(scanDest(b, &pkg, &halls, total, &b.StaffNotesCount)...); err != nil {
		return nil, err
	}
	b.PackageServiceIDs = nonEmpty(pkg)
	b.HallIDs = nonEmpty(halls)
	return b, nil
}

func (r *BookingRepo) GetByID(ctx context.Context, id string) (*domain.Booking, error) {
	q := `SELECT` + selectCols + ` FROM bookings WHERE id = $1`
	b, err := scanBooking(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("booking not found")
	}
	return b, err
}

func (r *BookingRepo) GetByIDs(ctx context.Context, ids []string) ([]*domain.Booking, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := `SELECT` + selectCols + ` FROM bookings WHERE id = ANY($1::uuid[])`
	rows, err := r.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Booking
	for rows.Next() {
		b, err := scanBookingRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *BookingRepo) ListByUser(ctx context.Context, userID, status string, offset, limit int) ([]*domain.Booking, int, error) {
	// Single query: COUNT(*) OVER() avoids a second round-trip and eliminates the
	// race window between a separate COUNT and the data fetch.
	// Note: when offset >= total the result set is empty and total stays 0.
	// This is acceptable because the client already has the real total from the first page.
	q := `SELECT` + selectCols + `, COUNT(*) OVER() AS total FROM bookings WHERE user_id = $1`
	args := []any{userID}

	if status != "" {
		q += ` AND status = $2`
		args = append(args, status)
	}

	q += ` ORDER BY created_at DESC OFFSET $` + strconv.Itoa(len(args)+1) + ` LIMIT $` + strconv.Itoa(len(args)+2)
	args = append(args, offset, limit)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var (
		bookings []*domain.Booking
		total    int
	)
	for rows.Next() {
		b, err := scanBookingWithTotal(rows, &total)
		if err != nil {
			return nil, 0, err
		}
		bookings = append(bookings, b)
	}
	return bookings, total, rows.Err()
}

// ListByVenue implements keyset pagination on (date, time_from, id).
// cursor is an opaque token of the form "date|time_from|id" (values from the last row
// of the previous page). Empty cursor = first page.
// Returns (bookings, total, nextCursor, error).
// total is derived from COUNT(*) OVER() — no second round-trip.
// nextCursor is empty when there are no more rows.
//
// Keyset design note: when date filter is set, the cursor still carries date but it is
// redundant — WHERE date=$n already pins the day, so the row-value predicate
// (date,time_from,id) > (cDate,cTimeFrom,cID) degenerates to (time_from,id) > (cTimeFrom,cID).
// PostgreSQL's planner collapses this correctly, so correctness and index usage are unaffected.
// A two-query variant (no-date path vs date-pinned path with a (time_from,id) cursor) would
// be marginally cleaner but adds branching for negligible real-world gain at typical venue
// scale (≤10k bookings/venue). Revisit if profiling shows planner regressions on large venues.
func (r *BookingRepo) ListByVenue(ctx context.Context, venueID, status, date, dateFrom, dateTo, cursor string, limit int) ([]*domain.Booking, int, string, error) {
	args := []any{venueID}
	n := 1

	q := `SELECT` + selectCols + `, COUNT(*) OVER() AS total,
		(SELECT COUNT(*) FROM booking_staff_notes n WHERE n.booking_id = bookings.id) AS staff_notes_count
		FROM bookings WHERE venue_id = $1`

	if status != "" {
		n++
		q += ` AND status = $` + strconv.Itoa(n)
		args = append(args, status)
	}
	if date != "" {
		// Exact day takes precedence over the range bounds (Сегодня/Расписание).
		n++
		q += ` AND date = $` + strconv.Itoa(n)
		args = append(args, date)
	} else {
		if dateFrom != "" {
			n++
			q += ` AND date >= $` + strconv.Itoa(n)
			args = append(args, dateFrom)
		}
		if dateTo != "" {
			n++
			q += ` AND date <= $` + strconv.Itoa(n)
			args = append(args, dateTo)
		}
	}

	// Keyset seek: skip rows already delivered on previous pages.
	if cursor != "" {
		cDate, cTimeFrom, cID, err := r.decodeCursor(cursor)
		if err != nil {
			return nil, 0, "", pkgerr.InvalidArgument("invalid cursor")
		}
		// Row-value comparison on the composite key (date, time_from, id).
		// Uses the natural btree index on (venue_id, date, time_from) + pk fallback.
		n += 3
		q += ` AND (date, time_from, id) > ($` + strconv.Itoa(n-2) + `, $` + strconv.Itoa(n-1) + `::time, $` + strconv.Itoa(n) + `::uuid)`
		args = append(args, cDate, cTimeFrom, cID)
	}

	q += ` ORDER BY date, time_from, id LIMIT $` + strconv.Itoa(n+1)
	// Fetch one extra row to detect whether a next page exists without a second query.
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, "", err
	}
	defer rows.Close()

	var (
		bookings []*domain.Booking
		total    int
	)
	for rows.Next() {
		b, err := scanBookingWithTotalAndNotes(rows, &total)
		if err != nil {
			return nil, 0, "", err
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, "", err
	}

	// Trim the sentinel row and build next_cursor from the last real row.
	var nextCursor string
	if len(bookings) > limit {
		bookings = bookings[:limit]
		last := bookings[limit-1]
		nextCursor = r.encodeCursor(last.Date.Format("2006-01-02"), last.TimeFrom.String(), last.ID)
	}
	return bookings, total, nextCursor, nil
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

// ConfirmPayment атомарно переводит бронь payment_pending → confirmed.
//
// Один round-trip в обоих случаях (успех и идемпотентный повтор).
// CTE upd выполняет UPDATE только если:
//   - status = 'payment_pending'
//   - payment_id IS NULL или совпадает с $2
//
// Если upd пуст (RowsAffected=0) — UNION ALL читает текущую строку из основной
// таблицы с confirmed=false. Это позволяет различить сценарии без второго запроса:
//   - confirmed=true  → UPDATE сработал, бронь подтверждена
//   - confirmed=false, status=confirmed/completed/cancelled → идемпотент, publish пропускаем
//   - confirmed=false, status=payment_pending → payment_id не совпал → ErrPaymentMismatch
//   - строка не найдена вообще → pgx.ErrNoRows → NotFound
//
// NATS at-least-once гарантирует дубли payment.completed — второй вызов попадает
// в ветку confirmed/completed и возвращает err=nil без лишнего round-trip.
func (r *BookingRepo) ConfirmPayment(ctx context.Context, bookingID, paymentID, subject string) (*domain.Booking, bool, error) {
	// confirmed — булев флаг: true если UPDATE задел строку, false если читали как-есть.
	// Добавляется последней колонкой после selectCols; scanDest принимает его через extra.
	const q = `
		WITH upd AS (
			UPDATE bookings
			SET status      = 'confirmed',
			    payment_id  = COALESCE(payment_id, $2::uuid),
			    updated_at  = now()
			WHERE id = $1::uuid
			  AND status = 'payment_pending'
			  AND (payment_id IS NULL OR payment_id = $2::uuid)
			RETURNING ` + selectCols + `, true AS confirmed
		)
		SELECT ` + selectCols + `, true  AS confirmed FROM upd
		UNION ALL
		SELECT ` + selectCols + `, false AS confirmed
		  FROM bookings
		 WHERE id = $1::uuid
		   AND NOT EXISTS (SELECT 1 FROM upd)`

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, q, bookingID, paymentID)
	if err != nil {
		return nil, false, err
	}

	if !rows.Next() {
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			return nil, false, rowsErr
		}
		return nil, false, pkgerr.NotFound("booking not found")
	}

	var confirmed bool
	b, scanErr := scanBookingFromRows(rows, &confirmed)
	rowsErr := rows.Err()
	// Закрываем rows ДО любого другого запроса в этой же транзакции (pgx не допускает
	// конкурентные запросы на одном соединении).
	rows.Close()
	if scanErr != nil {
		return nil, false, scanErr
	}
	if rowsErr != nil {
		return nil, false, rowsErr
	}

	if confirmed {
		// UPDATE сработал — бронь только что подтверждена. Пишем событие в той же tx.
		ev, err := domain.NewBookingEvent(subject, b)
		if err != nil {
			return nil, false, err
		}
		if err := insertOutboxTx(ctx, tx, ev); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return b, true, nil
	}
	// UPDATE не задел строку — события нет. Коммитим (запись не велась) и
	// смотрим текущий статус чтобы различить сценарии.
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	switch b.Status {
	case domain.StatusConfirmed, domain.StatusCompleted, domain.StatusCancelled:
		return b, false, nil // идемпотентный повтор — publish пропускаем
	default:
		// status=payment_pending, но payment_id не совпал.
		// Возможный сценарий: refund+repay в payment-service выпустил новый payment_id,
		// тогда как booking хранит предыдущий. Оба ID попадают в ошибку для разбора инцидента.
		return nil, false, pkgerr.InvalidArgument(fmt.Sprintf(
			"payment does not match booking: booking has payment_id=%s, got %s",
			b.PaymentID, paymentID,
		))
	}
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
	// completed_event_sent_at намеренно НЕ устанавливается здесь —
	// метка выставляется только после успешного NATS publish (MarkCompletedEventSent).
	// Это позволяет catch-up запросу найти строки, упавшие между UPDATE и publish.
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

// MarkCompletedEventSent выставляет completed_event_sent_at = now().
// Вызывается после каждого успешного PublishBookingCompleted.
func (r *BookingRepo) MarkCompletedEventSent(ctx context.Context, bookingID string) error {
	const q = `UPDATE bookings SET completed_event_sent_at = now() WHERE id = $1::uuid`
	_, err := r.pool.Exec(ctx, q, bookingID)
	return err
}

// FindUnpublishedCompleted возвращает брони status='completed' с completed_event_sent_at IS NULL.
// Покрывается partial-индексом idx_bookings_completed_unpublished.
func (r *BookingRepo) FindUnpublishedCompleted(ctx context.Context, limit int) ([]domain.BookingCompletedRef, error) {
	const q = `
		SELECT id::text, user_id::text, venue_id::text
		FROM bookings
		WHERE status = 'completed' AND completed_event_sent_at IS NULL
		ORDER BY updated_at
		LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
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

// DeleteOrphanPending удаляет брони в статусе pending без payment_id старше olderThanMinutes минут.
// Вызывается из AutoCompletePastVisits-тикера. Страхует от ситуации когда booking-service
// упал после repo.Create но до успешного repo.SetPaymentAndStatus + компенсационного repo.Delete:
// такие строки навсегда блокируют EXCLUDE-слот. 15 минут — консервативный порог,
// нормальный путь Create→SetPaymentAndStatus занимает секунды.
// См. TECH_DEBT [BOOKING-ORPHAN].
func (r *BookingRepo) DeleteOrphanPending(ctx context.Context, olderThanMinutes int) (int64, error) {
	const q = `
		DELETE FROM bookings
		WHERE status = 'pending'
		  AND payment_id IS NULL
		  AND created_at < now() - ($1 * INTERVAL '1 minute')`
	ct, err := r.pool.Exec(ctx, q, olderThanMinutes)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// nullableUUIDs возвращает nil когда слайс пустой — PG запишет NULL в uuid[].
// При непустом слайсе pgx кодирует []string как uuid[] через $N::uuid[].
func nullableUUIDs(ids []string) any {
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// nonEmpty фильтрует пустые строки из слайса, полученного при scan uuid[].
// COALESCE(col::text[], '{}') гарантирует что nil сюда не придёт,
// но пустой '{}'→[]string{} обрабатываем явно.
func nonEmpty(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// encodeCursor сериализует keyset-ключ последней строки страницы в непрозрачный токен.
// Формат: "date|time_from|id" ("|" не встречается ни в одном из полей).
// encodeCursor упаковывает позицию последней строки страницы в opaque-токен.
//
// Формат: base64url( payload "." mac16 )
// где payload = "date|time_from|id", mac16 = hex(HMAC-SHA256(key, payload)[:8]).
// Подпись предотвращает ручную сборку курсора клиентом; 8 байт достаточно
// для deterrence (не криптографическая граница — авторизация по venue_id в WHERE).
func (r *BookingRepo) encodeCursor(date, timeFrom, id string) string {
	payload := date + "|" + timeFrom + "|" + id
	mac := hmac.New(sha256.New, r.cursorHMACKey)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil)[:8])
	raw := payload + "." + sig
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor проверяет подпись и разбирает токен, созданный encodeCursor.
// Возвращает ошибку при невалидном base64, отсутствии mac или несовпадении подписи.
func (r *BookingRepo) decodeCursor(cursor string) (date, timeFrom, id string, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", "", fmt.Errorf("malformed cursor")
	}
	// Разбиваем по последней точке: всё до неё — payload, после — mac.
	dot := strings.LastIndexByte(string(raw), '.')
	if dot < 0 {
		return "", "", "", fmt.Errorf("malformed cursor: missing mac")
	}
	payload := string(raw[:dot])
	gotSig := string(raw[dot+1:])

	mac := hmac.New(sha256.New, r.cursorHMACKey)
	mac.Write([]byte(payload))
	wantSig := hex.EncodeToString(mac.Sum(nil)[:8])
	if !hmac.Equal([]byte(gotSig), []byte(wantSig)) {
		return "", "", "", fmt.Errorf("invalid cursor signature")
	}

	parts := strings.SplitN(payload, "|", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("malformed cursor: wrong payload structure")
	}
	return parts[0], parts[1], parts[2], nil
}
