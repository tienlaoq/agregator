-- Migration policy: down migrations are provided for local development and CI
-- rollback only. They are intentionally destructive (DROP COLUMN) and must
-- never be applied to a production database without a prior backup.
-- In production, prefer a forward-only migration strategy.
ALTER TABLE reviews
    DROP COLUMN IF EXISTS is_anonymous;
