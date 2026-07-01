-- Конвертируем TEXT (JSON-сериализованный массив UUID) → нативный uuid[].
--
-- Преимущества:
--   - Типобезопасность: PG отклонит невалидный UUID при вставке.
--   - Устраняет двойной JSON marshal/unmarshal в приложении.
--   - GIN-индекс на booking_hall_ids даёт эффективный поиск броней по залу
--     (оператор @>, ANY) без full-scan + парсинга текста.
--
-- ПОЧЕМУ ЧЕРЕЗ ФУНКЦИЮ: исходная версия встраивала подзапрос прямо в USING
-- (SELECT array_agg(...) FROM json_array_elements_text(...)), что PostgreSQL
-- отвергает ("cannot use subquery in transform expression"). Из-за этого
-- миграция не накатывалась с нуля (deploy/migrate.sh без ON_ERROR_STOP молча
-- её пропускал): колонки оставались TEXT, а репозиторий — который пишет
-- `$N::uuid[]` и сканирует uuid[] → []string — ломался на массивных полях.
-- Перенос конвертации в IMMUTABLE-функцию убирает подзапрос из USING.
--
-- Идемпотентность: конвертация защищена проверкой текущего типа колонки, а
-- функция/индекс создаются через OR REPLACE / IF NOT EXISTS, поэтому повторный
-- прогон (migrate.sh применяет каждый файл на каждом деплое) — no-op.

CREATE OR REPLACE FUNCTION json_text_to_uuid_array(p text)
    RETURNS uuid[]
    LANGUAGE sql
    IMMUTABLE
AS $$
    SELECT CASE
        WHEN p IS NULL OR btrim(p) = '' THEN NULL
        ELSE (
            SELECT array_agg(elem::uuid)
            FROM json_array_elements_text(p::json) AS elem
        )
    END;
$$;

DO $$
BEGIN
    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'bookings'
          AND column_name = 'package_service_ids') = 'text' THEN
        ALTER TABLE bookings
            ALTER COLUMN package_service_ids TYPE uuid[]
            USING json_text_to_uuid_array(package_service_ids);
    END IF;

    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'bookings'
          AND column_name = 'booking_hall_ids') = 'text' THEN
        ALTER TABLE bookings
            ALTER COLUMN booking_hall_ids TYPE uuid[]
            USING json_text_to_uuid_array(booking_hall_ids);
    END IF;
END $$;

-- GIN-индекс для поиска «все брони с залом X»:
--   SELECT * FROM bookings WHERE booking_hall_ids @> ARRAY['<hall-uuid>']::uuid[];
CREATE INDEX IF NOT EXISTS idx_bookings_hall_ids ON bookings USING GIN (booking_hall_ids);
