DROP INDEX IF EXISTS idx_users_active;
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
