ALTER TABLE masters
    ADD COLUMN IF NOT EXISTS yookassa_seller_account_id VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE master_bookings
    ADD COLUMN IF NOT EXISTS payment_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payment_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS total_price BIGINT NOT NULL DEFAULT 0;

ALTER TABLE master_bookings
    DROP CONSTRAINT IF EXISTS master_bookings_status_check;

ALTER TABLE master_bookings
    ADD CONSTRAINT master_bookings_status_check
        CHECK (status IN ('pending', 'payment_pending', 'confirmed', 'cancelled'));
