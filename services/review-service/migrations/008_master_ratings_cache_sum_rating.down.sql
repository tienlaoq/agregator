-- Destructive: local/CI rollback only. See 002_reviews_is_anonymous.down.sql for policy.
ALTER TABLE master_ratings_cache DROP COLUMN IF EXISTS sum_rating;
