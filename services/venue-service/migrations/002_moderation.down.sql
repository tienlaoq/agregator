DROP INDEX IF EXISTS idx_venues_status;
ALTER TABLE venues DROP COLUMN IF EXISTS moderation_comment;
ALTER TABLE venues DROP COLUMN IF EXISTS status;
