-- P1#11: chat_thread_participants is the single source of truth for participant
-- membership; the chat_threads.participant_user_ids TEXT[] column is a stale
-- denormalisation.  Drop it to eliminate the dual-write and the GIN index that
-- was never used for reads (repository always re-fetches from the participants
-- table anyway).

DROP INDEX IF EXISTS idx_chat_threads_participants;
ALTER TABLE chat_threads DROP COLUMN IF EXISTS participant_user_ids;
