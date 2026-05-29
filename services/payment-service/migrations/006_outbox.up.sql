-- Outbox table for reliable event publishing.
-- Rows are written atomically in the same transaction as the payment status UPDATE.
-- A background worker reads pending rows, publishes to NATS JetStream, and marks them sent.

CREATE TABLE IF NOT EXISTS payment_outbox (
    id              BIGSERIAL       PRIMARY KEY,
    -- The NATS subject to publish to, e.g. "payment.completed"
    subject         VARCHAR(128)    NOT NULL,
    -- JSON payload (same shape as the existing paymentEvent struct)
    payload         JSONB           NOT NULL,
    -- Populated from payments.id for correlation / debugging
    payment_id      UUID            NOT NULL,

    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    -- Null until the worker successfully publishes
    sent_at         TIMESTAMPTZ,
    -- Incremented on each failed publish attempt; worker backs off exponentially
    attempt_count   INT             NOT NULL DEFAULT 0,
    -- Null until a permanent failure is recorded (e.g. max attempts exceeded)
    failed_at       TIMESTAMPTZ,
    -- Last error message for debugging; never exposes PII
    last_error      TEXT
);

-- The worker polls for unsent, non-failed rows ordered by id.
CREATE INDEX IF NOT EXISTS payment_outbox_pending_idx
    ON payment_outbox (id)
    WHERE sent_at IS NULL AND failed_at IS NULL;
