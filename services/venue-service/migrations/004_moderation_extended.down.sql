DROP INDEX IF EXISTS idx_moderation_history_created;
DROP INDEX IF EXISTS idx_moderation_history_venue;
DROP TABLE IF EXISTS venue_moderation_history;
ALTER TABLE venues DROP COLUMN IF EXISTS moderated_by;
ALTER TABLE venues DROP COLUMN IF EXISTS moderated_at;
