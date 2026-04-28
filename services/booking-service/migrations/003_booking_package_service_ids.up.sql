-- Несколько пакетных услуг в одной брони: JSON-массив UUID (TEXT).
-- При одной услуге по-прежнему заполняется только service_id (legacy).
ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS package_service_ids TEXT;
