package domain

import "context"

type BookingRepository interface {
	Create(ctx context.Context, b *Booking) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*Booking, error)
	// GetByIDs fetches multiple bookings in a single query. Unknown ids are omitted.
	GetByIDs(ctx context.Context, ids []string) ([]*Booking, error)
	// ListByUser returns bookings for a user with offset-based pagination.
	// total comes from COUNT(*) OVER() — single round-trip.
	ListByUser(ctx context.Context, userID, status string, offset, limit int) ([]*Booking, int, error)
	// ListByVenue returns bookings for a venue using keyset pagination.
	// cursor encodes the exclusive lower-bound as "date|time_from|id"; empty = first page.
	// date filters an exact day; when empty, the inclusive [dateFrom, dateTo]
	// range applies (either bound may be empty for an open range).
	// total comes from COUNT(*) OVER() — single round-trip.
	// nextCursor is empty when no more rows remain.
	ListByVenue(ctx context.Context, venueID, status, date, dateFrom, dateTo, cursor string, limit int) ([]*Booking, int, string, error)
	UpdateStatus(ctx context.Context, id, status string) error
	SetPaymentID(ctx context.Context, bookingID, paymentID string) error
	// SetPaymentAndStatus атомарно устанавливает payment_id, payment_url и status за один UPDATE
	// и в ТОЙ ЖЕ транзакции пишет событие ev в outbox (booking.created).
	// Заменяет последовательные вызовы SetPaymentID + UpdateStatus в CreateBooking,
	// устраняя окно где бронь имеет payment_id без обновлённого статуса или наоборот.
	// payment_url персистируется в БД чтобы GetBooking возвращал его после перезапуска сервиса.
	// См. TECH_DEBT [BOOKING-PUBLISH-LOSS].
	SetPaymentAndStatus(ctx context.Context, bookingID, paymentID, paymentURL, status string, ev OutboxEvent) error
	// CancelWithEvent переводит бронь в status='cancelled' и в той же транзакции пишет
	// событие ev в outbox (booking.cancelled). Гарантии при отмене (CanCancel/дедлайн)
	// проверяются в usecase; здесь — безусловный UPDATE по id + INSERT в outbox.
	CancelWithEvent(ctx context.Context, id string, ev OutboxEvent) error
	// ConfirmPayment атомарно переводит бронь payment_pending → confirmed и
	// записывает payment_id (если ещё не записан) за один UPDATE … RETURNING.
	// При успешном переходе в ТОЙ ЖЕ транзакции пишет событие в outbox с subject.
	// confirmed=false означает что строка не была обновлена — бронь уже находится
	// в терминальном статусе (confirmed/completed/cancelled) или не существует;
	// в этом случае событие не пишется.
	// Возвращает ErrPaymentMismatch если payment_id уже записан и не совпадает с переданным.
	ConfirmPayment(ctx context.Context, bookingID, paymentID, subject string) (b *Booking, confirmed bool, err error)

	// FetchUnsentOutbox возвращает неотправленные события (sent_at IS NULL) в порядке id (FIFO).
	// limit ограничивает батч. Используется поллером ProcessOutbox.
	FetchUnsentOutbox(ctx context.Context, limit int) ([]OutboxEvent, error)
	// MarkOutboxSent проставляет sent_at = now() для указанных id. Вызывается ПОСЛЕ
	// успешной публикации (publish раньше mark = at-least-once). Пустой срез — no-op.
	MarkOutboxSent(ctx context.Context, ids []int64) error
	HasCompleted(ctx context.Context, userID, venueID string) (bool, error)
	// AutoCompleteVisitEnded переводит confirmed → completed если конец слота уже прошёл.
	// completed_event_sent_at намеренно остаётся NULL — метка выставляется после успешного publish.
	AutoCompleteVisitEnded(ctx context.Context, visitTimeZone string) ([]BookingCompletedRef, error)
	// MarkCompletedEventSent выставляет completed_event_sent_at = now() после успешного publish.
	// Идемпотентен: повторный вызов просто обновит временную метку.
	MarkCompletedEventSent(ctx context.Context, bookingID string) error
	// FindUnpublishedCompleted возвращает брони в статусе completed
	// у которых completed_event_sent_at IS NULL (событие не было опубликовано).
	// Используется catch-up поллером для восстановления после сбоя между UPDATE и publish.
	// limit ограничивает батч (рекомендуется 100).
	FindUnpublishedCompleted(ctx context.Context, limit int) ([]BookingCompletedRef, error)

	// DeleteOrphanPending удаляет брони в статусе pending без payment_id старше olderThan минут.
	// Это «зависшие» записи: booking-service упал между repo.Create и repo.SetPaymentAndStatus
	// и не успел выполнить компенсационный repo.Delete. Такие строки блокируют EXCLUDE-слот
	// навсегда. Возвращает количество удалённых строк.
	// См. TECH_DEBT [BOOKING-ORPHAN].
	DeleteOrphanPending(ctx context.Context, olderThanMinutes int) (int64, error)

	ListBookingStaffNotes(ctx context.Context, bookingID string) ([]BookingStaffNote, error)
	AddBookingStaffNote(ctx context.Context, n *BookingStaffNote) error
}

// BookingCompletedRef — строки, переведённые в completed (для событий).
type BookingCompletedRef struct {
	ID      string
	UserID  string
	VenueID string
}
