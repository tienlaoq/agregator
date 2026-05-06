DROP INDEX IF EXISTS uq_chat_messages_client_msg;

ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS client_msg_id;

DROP INDEX IF EXISTS idx_chat_thread_participants_user_active;

DROP TABLE IF EXISTS chat_thread_participants;

