DROP TABLE IF EXISTS chat_reads;
ALTER TABLE IF EXISTS chat_threads DROP CONSTRAINT IF EXISTS fk_chat_threads_last_message;
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS chat_threads;
