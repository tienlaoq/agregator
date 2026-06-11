-- Витрина продуктовых событий с фронтенда (analytics.web).
-- props уже отфильтрованы серверным whitelist'ом в api-gateway
-- (handler/analytics.go) — PII сюда не попадает by construction.
CREATE TABLE IF NOT EXISTS events (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- stream_seq — позиция сообщения в JetStream-стриме ANALYTICS.
    -- UNIQUE превращает повторную доставку (at-least-once) в no-op.
    stream_seq BIGINT NOT NULL UNIQUE,
    ts         TIMESTAMPTZ NOT NULL DEFAULT now(),
    event      TEXT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    props      JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_events_ts ON events (ts);
CREATE INDEX IF NOT EXISTS idx_events_event_ts ON events (event, ts);
