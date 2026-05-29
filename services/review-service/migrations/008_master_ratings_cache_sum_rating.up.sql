-- Symmetric incremental-rating support for master_ratings_cache.
ALTER TABLE master_ratings_cache
    ADD COLUMN IF NOT EXISTS sum_rating FLOAT NOT NULL DEFAULT 0;

UPDATE master_ratings_cache mrc
SET sum_rating = (
    SELECT COALESCE(SUM(rating), 0)
    FROM reviews
    WHERE master_id = mrc.master_id
);
