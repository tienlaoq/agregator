CREATE TABLE IF NOT EXISTS reviews (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL,
    venue_id   UUID NOT NULL,
    rating     INT  NOT NULL CHECK (rating BETWEEN 1 AND 5),
    text       TEXT,
    is_verified BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT now(),

    CONSTRAINT uq_user_venue UNIQUE (user_id, venue_id)
);

CREATE INDEX idx_reviews_venue_created ON reviews (venue_id, created_at DESC);

CREATE TABLE IF NOT EXISTS ratings_cache (
    venue_id     UUID PRIMARY KEY,
    avg_rating   FLOAT DEFAULT 0,
    review_count INT   DEFAULT 0,
    updated_at   TIMESTAMPTZ DEFAULT now()
);
