-- Add locked_at to support the claim-then-publish outbox pattern.
-- A worker atomically sets locked_at = now() inside a short transaction
-- (no NATS I/O), then publishes outside any DB transaction.
-- A background sweep resets locks that are older than the stale threshold
-- so that crashes / NATS timeouts do not permanently orphan rows.

ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ;

-- Unpublished rows that are either unlocked or have a stale lock.
-- Used by ClaimBatch; replaces the old idx_outbox_unpublished.
DROP INDEX IF EXISTS idx_outbox_unpublished;

CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON outbox_events (created_at)
    WHERE published_at IS NULL;
