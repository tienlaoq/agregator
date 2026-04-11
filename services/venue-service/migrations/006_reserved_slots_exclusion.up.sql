-- Prevent overlapping reservations for the same venue on the same calendar day.
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE reserved_slots
  ADD COLUMN IF NOT EXISTS slot_ts TSRANGE GENERATED ALWAYS AS (
    tsrange(
      (date + time_from)::timestamp,
      (date + time_to)::timestamp,
      '[)'
    )
  ) STORED;

ALTER TABLE reserved_slots DROP CONSTRAINT IF EXISTS reserved_slots_venue_id_date_time_from_key;

ALTER TABLE reserved_slots DROP CONSTRAINT IF EXISTS reserved_slots_no_overlap;

ALTER TABLE reserved_slots ADD CONSTRAINT reserved_slots_no_overlap
  EXCLUDE USING gist (venue_id WITH =, slot_ts WITH &&);
