CREATE TABLE IF NOT EXISTS chat_threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL,
    ref_id UUID NOT NULL,
    participant_user_ids TEXT[] NOT NULL DEFAULT '{}',
    last_message_id UUID NULL,
    last_message_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kind, ref_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_threads_participants ON chat_threads USING GIN (participant_user_ids);
CREATE INDEX IF NOT EXISTS idx_chat_threads_last_message_at ON chat_threads (last_message_at DESC);

CREATE TABLE IF NOT EXISTS chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    author_user_id UUID NOT NULL,
    text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_thread_created ON chat_messages (thread_id, created_at ASC);

ALTER TABLE chat_threads
    ADD CONSTRAINT fk_chat_threads_last_message
    FOREIGN KEY (last_message_id) REFERENCES chat_messages(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS chat_reads (
    thread_id UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    last_read_at TIMESTAMPTZ NULL,
    PRIMARY KEY (thread_id, user_id)
);

