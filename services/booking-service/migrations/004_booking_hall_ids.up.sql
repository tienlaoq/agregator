-- Выбранные залы в брони (JSON-массив UUID).
ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS booking_hall_ids TEXT;
