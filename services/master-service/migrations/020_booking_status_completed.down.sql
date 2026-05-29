ALTER TABLE master_bookings
    DROP CONSTRAINT IF EXISTS master_bookings_status_check;

ALTER TABLE master_bookings
    ADD CONSTRAINT master_bookings_status_check
        CHECK (status IN ('pending', 'payment_pending', 'confirmed', 'cancelled'));
