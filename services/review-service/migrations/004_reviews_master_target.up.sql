ALTER TABLE reviews
ADD COLUMN IF NOT EXISTS master_id UUID;

ALTER TABLE reviews
ALTER COLUMN venue_id DROP NOT NULL;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.table_constraints
    WHERE table_name = 'reviews' AND constraint_name = 'uq_user_venue'
  ) THEN
    ALTER TABLE reviews DROP CONSTRAINT uq_user_venue;
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_reviews_user_venue
ON reviews (user_id, venue_id)
WHERE venue_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_reviews_user_master
ON reviews (user_id, master_id)
WHERE master_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_reviews_master_created
ON reviews (master_id, created_at DESC)
WHERE master_id IS NOT NULL;

ALTER TABLE reviews
DROP CONSTRAINT IF EXISTS reviews_exactly_one_target;

ALTER TABLE reviews
ADD CONSTRAINT reviews_exactly_one_target
CHECK (
  (venue_id IS NOT NULL AND master_id IS NULL)
  OR
  (venue_id IS NULL AND master_id IS NOT NULL)
);
