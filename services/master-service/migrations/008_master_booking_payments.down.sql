ALTER TABLE master_bookings
    DROP CONSTRAINT IF EXISTS master_bookings_status_check;

ALTER TABLE master_bookings
    ADD CONSTRAINT master_bookings_status_check
        CHECK (status IN ('pending', 'confirmed', 'cancelled'));

ALTER TABLE master_bookings
    DROP COLUMN IF EXISTS total_price,
    DROP COLUMN IF EXISTS payment_url,
    DROP COLUMN IF EXISTS payment_id;

ALTER TABLE masters
    DROP COLUMN IF EXISTS yookassa_seller_account_id;
