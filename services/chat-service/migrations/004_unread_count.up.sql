-- P1#10: denormalise unread_count into chat_thread_participants so that
-- ListThreadsForUser no longer needs a correlated subquery per thread.
--
-- The column is updated by two triggers:
--   trg_increment_unread  — fired after INSERT on chat_messages
--   trg_reset_unread      — fired after UPSERT on chat_reads (MarkRead)

ALTER TABLE chat_thread_participants
    ADD COLUMN IF NOT EXISTS unread_count INT NOT NULL DEFAULT 0;

-- Back-fill: count messages not yet read by each participant.
UPDATE chat_thread_participants tp
SET unread_count = (
    SELECT COUNT(*)
    FROM   chat_messages m
    LEFT JOIN chat_reads cr
           ON cr.thread_id = m.thread_id
          AND cr.user_id   = tp.user_id
    WHERE  m.thread_id       = tp.thread_id
      AND  m.author_user_id <> tp.user_id
      AND  (
               cr.user_id IS NULL
            OR cr.last_read_message_id IS NULL
            OR EXISTS (
                   SELECT 1
                   FROM   chat_messages wm
                   WHERE  wm.id = cr.last_read_message_id
                     AND (
                           m.created_at > wm.created_at
                        OR (m.created_at = wm.created_at AND m.id > wm.id)
                         )
               )
           )
);

-- Trigger: increment unread for every participant except the author.
CREATE OR REPLACE FUNCTION fn_increment_unread() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    UPDATE chat_thread_participants
    SET    unread_count = unread_count + 1
    WHERE  thread_id = NEW.thread_id
      AND  user_id  <> NEW.author_user_id
      AND  left_at IS NULL;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_increment_unread ON chat_messages;
CREATE TRIGGER trg_increment_unread
    AFTER INSERT ON chat_messages
    FOR EACH ROW EXECUTE FUNCTION fn_increment_unread();

-- Trigger: reset unread to 0 for the user who just called MarkRead.
CREATE OR REPLACE FUNCTION fn_reset_unread() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    UPDATE chat_thread_participants
    SET    unread_count = 0
    WHERE  thread_id = NEW.thread_id
      AND  user_id   = NEW.user_id;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS trg_reset_unread ON chat_reads;
CREATE TRIGGER trg_reset_unread
    AFTER INSERT OR UPDATE ON chat_reads
    FOR EACH ROW EXECUTE FUNCTION fn_reset_unread();
