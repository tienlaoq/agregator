-- Mobile push tokens: one row per (device) FCM registration token. A user may
-- have several (phone + tablet); a single token belongs to one user, so token
-- is the primary key and re-registering under a new user reassigns it.
CREATE TABLE IF NOT EXISTS device_tokens (
    token      TEXT PRIMARY KEY,
    user_id    UUID NOT NULL,
    platform   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fan-out lookup: all tokens for a user when a notification is created.
CREATE INDEX IF NOT EXISTS idx_device_tokens_user
    ON device_tokens (user_id);
