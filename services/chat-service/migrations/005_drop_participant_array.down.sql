ALTER TABLE chat_threads ADD COLUMN IF NOT EXISTS participant_user_ids TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX IF NOT EXISTS idx_chat_threads_participants ON chat_threads USING GIN (participant_user_ids);

-- Re-populate from the participants table.
UPDATE chat_threads t
SET participant_user_ids = (
    SELECT ARRAY(
        SELECT user_id::text FROM chat_thread_participants
        WHERE thread_id = t.id AND left_at IS NULL ORDER BY joined_at
    )
);
