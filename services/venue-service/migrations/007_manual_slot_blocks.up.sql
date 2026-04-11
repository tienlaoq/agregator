-- Owner-marked busy intervals (external bookings): booking_id NULL, same overlap rules as aggregator bookings.
ALTER TABLE reserved_slots ALTER COLUMN booking_id DROP NOT NULL;

ALTER TABLE reserved_slots
  ADD COLUMN IF NOT EXISTS block_note TEXT;

COMMENT ON COLUMN reserved_slots.booking_id IS 'Booking from aggregator; NULL = manual owner block';
COMMENT ON COLUMN reserved_slots.block_note IS 'Optional note for manual blocks (e.g. phone booking)';
