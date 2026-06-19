-- Transactional outbox для событий booking.* (TECH_DEBT [BOOKING-PUBLISH-LOSS]).
--
-- Проблема: booking.created/confirmed/cancelled публиковались напрямую в NATS
-- после коммита в БД. При недоступном NATS событие терялось навсегда — рефанд
-- при отмене не инициировался, уведомление о подтверждении не уходило.
--
-- Решение: смена статуса брони и запись события происходят в ОДНОЙ транзакции
-- (INSERT в эту таблицу внутри той же tx, что и UPDATE bookings). Выделенный
-- поллер читает неотправленные строки, публикует в NATS и проставляет sent_at.
-- Publish раньше mark = at-least-once; консьюмеры (review/notification)
-- идемпотентны по booking_id.
--
-- booking.completed использует отдельный механизм (completed_event_sent_at,
-- migration 007) и в этот outbox НЕ входит.

CREATE TABLE outbox_events (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    subject    TEXT        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ
);

-- Partial-индекс покрывает запрос поллера (WHERE sent_at IS NULL ORDER BY id).
-- Не растёт с историей отправленных событий.
CREATE INDEX idx_outbox_unsent ON outbox_events (id) WHERE sent_at IS NULL;
