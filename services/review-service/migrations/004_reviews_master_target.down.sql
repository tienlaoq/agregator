-- Destructive: local/CI rollback only. See 002_reviews_is_anonymous.down.sql for policy.
ALTER TABLE reviews
    DROP CONSTRAINT IF EXISTS reviews_exactly_one_target;

DROP INDEX IF EXISTS idx_reviews_master_created;
DROP INDEX IF EXISTS uq_reviews_user_master;
DROP INDEX IF EXISTS uq_reviews_user_venue;

ALTER TABLE reviews
    ADD CONSTRAINT uq_user_venue UNIQUE (user_id, venue_id);

ALTER TABLE reviews
    ALTER COLUMN venue_id SET NOT NULL;

ALTER TABLE reviews
    DROP COLUMN IF EXISTS master_id;
