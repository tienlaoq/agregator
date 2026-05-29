DROP TRIGGER IF EXISTS trg_reset_unread ON chat_reads;
DROP TRIGGER IF EXISTS trg_increment_unread ON chat_messages;
DROP FUNCTION IF EXISTS fn_reset_unread();
DROP FUNCTION IF EXISTS fn_increment_unread();
ALTER TABLE chat_thread_participants DROP COLUMN IF EXISTS unread_count;
