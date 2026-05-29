-- Предотвращает двойную бронь одного слота в одном заведении на уровне БД.
-- Статусы 'cancelled' исключены — отменённые брони слот не занимают.
--
-- Требует расширения btree_gist (включено по умолчанию в большинстве managed PG).
-- tsrange строится из date + time_from/time_to: перекрытие ловит && на gist-индексе.

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
