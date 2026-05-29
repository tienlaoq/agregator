-- Revert: remove 'refunded' from status CHECK constraint.
-- WARNING: if any rows have status='refunded', this will fail.
-- Run: UPDATE payments SET status='cancelled' WHERE status='refunded';
-- before applying this rollback in production.
ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN ('pending', 'succeeded', 'cancelled'));
