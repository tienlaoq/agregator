CREATE TABLE IF NOT EXISTS bookings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    venue_id    UUID NOT NULL,
    service_id  UUID,
    date        DATE NOT NULL,
    time_from   TIME NOT NULL,
    time_to     TIME NOT NULL,
    guests      INT DEFAULT 1,
    comment     TEXT,
    status      VARCHAR(50) NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'payment_pending', 'confirmed', 'completed', 'cancelled')),
    total_price BIGINT DEFAULT 0,
    payment_id  UUID,
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_bookings_user_status ON bookings (user_id, status);
CREATE INDEX idx_bookings_venue_date ON bookings (venue_id, date);
