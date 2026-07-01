-- Limit moderation comment length to 1000 characters.
--
-- Rationale (152-FZ / data minimisation):
--   master_moderation_history.comment is stored indefinitely and visible to
--   all admins via ListModerationHistory. Without a length cap a moderator can
--   accidentally paste personal data (passport scan text, phone numbers, etc.)
--   into a free-form note. 1000 chars is sufficient for any legitimate reason
--   for approval / rejection / revision request.
--
--   The same constraint is added to masters.moderation_comment so the two
--   columns stay consistent (the column is copied into history on every status
--   change).

-- DROP-then-ADD makes this idempotent: deploy/migrate.sh re-applies every
-- *.up.sql on each deploy, and PostgreSQL has no ADD CONSTRAINT IF NOT EXISTS.
ALTER TABLE master_moderation_history
    DROP CONSTRAINT IF EXISTS chk_moderation_history_comment_length;
ALTER TABLE master_moderation_history
    ADD CONSTRAINT chk_moderation_history_comment_length
        CHECK (char_length(comment) <= 1000);

ALTER TABLE masters
    DROP CONSTRAINT IF EXISTS chk_masters_moderation_comment_length;
ALTER TABLE masters
    ADD CONSTRAINT chk_masters_moderation_comment_length
        CHECK (char_length(moderation_comment) <= 1000);
