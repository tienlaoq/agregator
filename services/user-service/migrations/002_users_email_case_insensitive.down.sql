DROP INDEX IF EXISTS idx_users_email_lower;

-- Restore the original case-sensitive constraint.
-- Note: this will fail if the table already contains rows that differ only
-- in case — clean them up manually before rolling back.
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
