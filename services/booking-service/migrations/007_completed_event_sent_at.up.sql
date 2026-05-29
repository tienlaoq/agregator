-- Метка успешной публикации события booking.completed.
--
-- Проблема: AutoCompletePastVisits делает UPDATE status='completed' атомарно,
-- но после падения процесса между UPDATE и NATS publish событие теряется —
-- бронь уже completed, повторный поллер её не тронет.
--
-- Решение: completed_event_sent_at = NULL пока событие не опубликовано.
-- После успешного publish → UPDATE SET completed_event_sent_at = now().
-- Catch-up запрос в том же поллере находит completed + sent_at IS NULL
-- и повторно публикует — идемпотентно для downstream (review/notification
-- сервисы должны быть идемпотентны по booking_id).
--
-- Индекс покрывает catch-up запрос: WHERE status='completed' AND completed_event_sent_at IS NULL.
-- Partial index — только незакрытые строки, не растёт с историей.

ALTER TABLE bookings
    ADD COLUMN completed_event_sent_at TIMESTAMPTZ;

CREATE INDEX idx_bookings_completed_unpublished
    ON bookings (id)
    WHERE status = 'completed' AND completed_event_sent_at IS NULL;
