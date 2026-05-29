-- Add sum_rating to ratings_cache so avg can be maintained incrementally (O(1) per review).
-- Backfill from the reviews table so existing rows stay correct after migration.
ALTER TABLE ratings_cache
    ADD COLUMN IF NOT EXISTS sum_rating FLOAT NOT NULL DEFAULT 0;

UPDATE ratings_cache rc
SET sum_rating = (
    SELECT COALESCE(SUM(rating), 0)
    FROM reviews
    WHERE venue_id = rc.venue_id
);
