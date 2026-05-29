CREATE TABLE IF NOT EXISTS outbox_events (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_id UUID        NOT NULL,
    event_type   TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

-- Worker polls unpublished rows; partial index keeps the scan cheap.
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished
    ON outbox_events (created_at)
    WHERE published_at IS NULL;
