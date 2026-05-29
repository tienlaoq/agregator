-- Конвертируем TEXT (JSON-сериализованный массив UUID) → нативный uuid[].
--
-- Преимущества:
--   - Типобезопасность: PG отклонит невалидный UUID при вставке.
--   - Устраняет двойной JSON marshal/unmarshal в приложении.
--   - GIN-индекс на booking_hall_ids даёт эффективный поиск броней по залу
--     (оператор @>, ANY) без full-scan + парсинга текста.
--
-- USING-выражение обрабатывает три случая хранящихся данных:
--   NULL                     → NULL
--   ''  (пустая строка)      → NULL
--   '["uuid1","uuid2",...]'  → ARRAY[uuid1, uuid2, ...]::uuid[]
--
-- Конвертация: json_array_elements_text разворачивает JSON-массив в строки,
-- array_agg собирает обратно, ::uuid[] валидирует каждый элемент.

ALTER TABLE bookings
    ALTER COLUMN package_service_ids
    TYPE uuid[]
    USING (
        CASE
            WHEN package_service_ids IS NULL OR trim(package_service_ids) = ''
            THEN NULL
            ELSE (
                SELECT array_agg(v::uuid)
                FROM json_array_elements_text(package_service_ids::json) AS v
            )
        END
    );

ALTER TABLE bookings
    ALTER COLUMN booking_hall_ids
    TYPE uuid[]
    USING (
        CASE
            WHEN booking_hall_ids IS NULL OR trim(booking_hall_ids) = ''
            THEN NULL
            ELSE (
                SELECT array_agg(v::uuid)
                FROM json_array_elements_text(booking_hall_ids::json) AS v
            )
        END
    );

-- GIN-индекс для поиска «все брони с залом X»:
--   SELECT * FROM bookings WHERE booking_hall_ids @> ARRAY['<hall-uuid>']::uuid[];
CREATE INDEX IF NOT EXISTS idx_bookings_hall_ids ON bookings USING GIN (booking_hall_ids);
