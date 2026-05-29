CREATE TABLE IF NOT EXISTS master_ratings_cache (
    master_id    UUID        PRIMARY KEY,
    avg_rating   FLOAT       NOT NULL DEFAULT 0,
    review_count INT         NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
