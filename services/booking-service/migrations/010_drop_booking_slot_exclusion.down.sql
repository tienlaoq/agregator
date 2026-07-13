-- Восстанавливает venue-wide EXCLUDE из миграции 005 (откат 010).
-- ВНИМАНИЕ: ломает per-hall режим — см. 010_drop_booking_slot_exclusion.up.sql.
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE bookings
    ADD CONSTRAINT bookings_no_overlap
    EXCLUDE USING gist (
        venue_id   WITH =,
        tsrange(
            (date + time_from)::timestamp,
            (date + time_to)::timestamp,
            '[)'
        ) WITH &&
    )
    WHERE (status NOT IN ('cancelled'));
