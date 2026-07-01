-- Откат: uuid[] → TEXT (JSON-массив, как было до миграции 006).
DROP INDEX IF EXISTS idx_bookings_hall_ids;

ALTER TABLE bookings
    ALTER COLUMN booking_hall_ids
    TYPE TEXT
    USING (
        CASE
            WHEN booking_hall_ids IS NULL THEN NULL
            ELSE array_to_json(booking_hall_ids::text[])::text
        END
    );

ALTER TABLE bookings
    ALTER COLUMN package_service_ids
    TYPE TEXT
    USING (
        CASE
            WHEN package_service_ids IS NULL THEN NULL
            ELSE array_to_json(package_service_ids::text[])::text
        END
    );

DROP FUNCTION IF EXISTS json_text_to_uuid_array(text);
