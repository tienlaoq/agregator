CREATE TABLE IF NOT EXISTS payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id      UUID NOT NULL UNIQUE,
    amount          BIGINT NOT NULL,
    status          VARCHAR(50) DEFAULT 'pending'
                    CHECK (status IN ('pending', 'succeeded', 'cancelled')),
    provider_id     VARCHAR(255),
    payment_url     TEXT,
    idempotency_key VARCHAR(255) UNIQUE,
    created_at      TIMESTAMPTZ DEFAULT now(),
    updated_at      TIMESTAMPTZ DEFAULT now()
);
