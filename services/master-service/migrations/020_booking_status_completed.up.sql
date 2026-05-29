-- 3.21 fix: add 'completed' to master_bookings status CHECK constraint.
--
-- The original constraint from migration 008 allowed only:
--   ('pending', 'payment_pending', 'confirmed', 'cancelled')
-- HasCompletedBookingByClientMaster queried status = 'completed', which
-- could never match → reviews for master bookings were permanently blocked.
--
-- 'completed' represents a confirmed booking whose end time has passed.
-- Transition: confirmed → completed (either by a future cron-job that calls
-- UpdateBookingStatus, or implicitly recognised in HasCompletedBookingByClientMaster
-- by checking confirmed + time_to < now() until the cron-job is implemented).
ALTER TABLE master_bookings
    DROP CONSTRAINT IF EXISTS master_bookings_status_check;

ALTER TABLE master_bookings
    ADD CONSTRAINT master_bookings_status_check
        CHECK (status IN ('pending', 'payment_pending', 'confirmed', 'cancelled', 'completed'));
