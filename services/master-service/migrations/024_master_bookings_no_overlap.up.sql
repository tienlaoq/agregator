-- Предотвращает двойную бронь одного слота у одного мастера на уровне БД.
-- Статусы 'cancelled' исключены — отменённые брони слот не занимают.
-- Зеркалит booking-service migration 005 (bookings_no_overlap).
--
-- Требует btree_gist для сравнения master_id (uuid) WITH =.
-- tsrange строится из date + time_from/time_to; граница '[)' — смежные слоты
-- (одна бронь до 15:00, следующая с 15:00) не считаются пересечением.

CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE master_bookings
    ADD CONSTRAINT master_bookings_no_overlap
    EXCLUDE USING gist (
        master_id WITH =,
        tsrange(
            (date + time_from)::timestamp,
            (date + time_to)::timestamp,
            '[)'
        ) WITH &&
    )
    WHERE (status <> 'cancelled');
