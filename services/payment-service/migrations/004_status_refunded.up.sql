-- Bug fix: 'refunded' was missing from the status CHECK constraint.
-- RefundByBooking writes status='refunded' after a successful ЮKassa refund,
-- but the original constraint only allowed ('pending','succeeded','cancelled'),
-- causing every refund UPDATE to fail with a constraint violation in production.
--
-- PostgreSQL does not support ALTER TABLE … ALTER CHECK; we must drop and re-add.
-- The auto-generated constraint name from 001_init is payments_status_check.
ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS payments_status_check;

ALTER TABLE payments
    ADD CONSTRAINT payments_status_check
        CHECK (status IN ('pending', 'succeeded', 'cancelled', 'refunded'));
