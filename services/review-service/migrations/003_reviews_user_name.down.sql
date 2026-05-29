-- Destructive: local/CI rollback only. See 002_reviews_is_anonymous.down.sql for policy.
ALTER TABLE reviews
    DROP COLUMN IF EXISTS user_name;
