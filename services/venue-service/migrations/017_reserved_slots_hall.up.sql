-- Per-hall availability (mixed booking mode). Additive and behaviour-preserving:
-- existing rows keep hall_id NULL, so COALESCE(hall_id, venue_id) = venue_id and
-- the overlap rule stays venue-wide, exactly as before. A venue opts into
-- per-hall booking via venues.booking_mode = 'per_hall'.
ALTER TABLE venues
  ADD COLUMN IF NOT EXISTS booking_mode TEXT NOT NULL DEFAULT 'whole'
    CHECK (booking_mode IN ('whole', 'per_hall'));

ALTER TABLE reserved_slots
  ADD COLUMN IF NOT EXISTS hall_id UUID;

COMMENT ON COLUMN reserved_slots.hall_id IS
  'Reserved hall in per-hall mode; NULL = whole-venue reservation (mode=whole)';

-- Overlap is now keyed on the reserved resource: the specific hall when set,
-- otherwise the whole venue. btree_gist is already enabled (migration 006).
-- For legacy rows (hall_id NULL) this is identical to the previous venue-wide
-- constraint.
ALTER TABLE reserved_slots DROP CONSTRAINT IF EXISTS reserved_slots_no_overlap;
ALTER TABLE reserved_slots ADD CONSTRAINT reserved_slots_no_overlap
  EXCLUDE USING gist ((COALESCE(hall_id, venue_id)) WITH =, slot_ts WITH &&);

CREATE INDEX IF NOT EXISTS idx_reserved_slots_hall
  ON reserved_slots (venue_id, hall_id, date);
