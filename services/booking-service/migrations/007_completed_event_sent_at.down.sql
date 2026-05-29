DROP INDEX IF EXISTS idx_bookings_completed_unpublished;
ALTER TABLE bookings DROP COLUMN IF EXISTS completed_event_sent_at;
