ALTER TABLE chat_reads
    ADD COLUMN IF NOT EXISTS last_read_message_id UUID NULL
        REFERENCES chat_messages (id) ON DELETE SET NULL;
