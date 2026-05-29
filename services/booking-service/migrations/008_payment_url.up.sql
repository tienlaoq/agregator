-- payment_url хранится для отображения клиенту пока статус = payment_pending.
-- После перехода в confirmed/cancelled поле остаётся для аудита, но фронт его не показывает.
-- Аналогично master-service: payment_url персистируется в БД, а не только в памяти.
ALTER TABLE bookings ADD COLUMN payment_url TEXT;
