-- Indexes to support the periodic cleanup job (runTokenCleanup in cmd/main.go)
-- and existing queries that were previously doing full scans.
--
-- refresh_tokens:
--   • expires_at — used by DELETE WHERE expires_at < now() (cleanup job)
--   • user_id    — used by DELETE WHERE user_id = $1 (Logout, password change,
--                  reuse-detection revocation). Without this index every call
--                  to RefreshTokenRepo.DeleteByUserID is a full table scan.
--
-- password_reset_tokens:
--   • expires_at — used by DELETE WHERE expires_at < now() OR used_at IS NOT NULL

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at
    ON refresh_tokens (expires_at);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id
    ON refresh_tokens (user_id);

CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_expires_at
    ON password_reset_tokens (expires_at);
