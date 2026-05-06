CREATE TABLE IF NOT EXISTS chat_thread_participants (
    thread_id UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'participant',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at TIMESTAMPTZ NULL,
    PRIMARY KEY (thread_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_thread_participants_user_active
    ON chat_thread_participants (user_id, thread_id)
    WHERE left_at IS NULL;

INSERT INTO chat_thread_participants (thread_id, user_id, role)
SELECT t.id, p.uid::uuid, 'participant'
FROM chat_threads t
CROSS JOIN LATERAL unnest(t.participant_user_ids) AS p(uid)
ON CONFLICT (thread_id, user_id) DO NOTHING;

ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS client_msg_id TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_chat_messages_client_msg
    ON chat_messages (thread_id, author_user_id, client_msg_id)
    WHERE client_msg_id <> '';

