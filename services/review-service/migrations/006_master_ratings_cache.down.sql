-- Destructive: local/CI rollback only. See 002_reviews_is_anonymous.down.sql for policy.
DROP TABLE IF EXISTS master_ratings_cache;
