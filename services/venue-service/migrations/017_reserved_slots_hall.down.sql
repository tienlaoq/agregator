-- Restore the venue-wide overlap constraint. NOTE: if any per-hall reservations
-- with overlapping times exist in the same venue, this will fail — release them
-- first. hall_id data is dropped.
ALTER TABLE reserved_slots DROP CONSTRAINT IF EXISTS reserved_slots_no_overlap;
ALTER TABLE reserved_slots ADD CONSTRAINT reserved_slots_no_overlap
  EXCLUDE USING gist (venue_id WITH =, slot_ts WITH &&);

DROP INDEX IF EXISTS idx_reserved_slots_hall;
ALTER TABLE reserved_slots DROP COLUMN IF EXISTS hall_id;
ALTER TABLE venues DROP COLUMN IF EXISTS booking_mode;
