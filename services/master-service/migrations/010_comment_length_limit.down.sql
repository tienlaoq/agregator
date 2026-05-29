ALTER TABLE master_moderation_history
    DROP CONSTRAINT IF EXISTS chk_moderation_history_comment_length;

ALTER TABLE masters
    DROP CONSTRAINT IF EXISTS chk_masters_moderation_comment_length;
