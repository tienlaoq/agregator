ALTER TABLE reserved_slots DROP CONSTRAINT IF EXISTS reserved_slots_no_overlap;

ALTER TABLE reserved_slots DROP COLUMN IF EXISTS slot_ts;

ALTER TABLE reserved_slots
  ADD CONSTRAINT reserved_slots_venue_id_date_time_from_key
  UNIQUE (venue_id, date, time_from);
