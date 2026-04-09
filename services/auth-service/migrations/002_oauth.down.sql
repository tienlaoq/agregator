DROP INDEX IF EXISTS idx_credentials_provider;
ALTER TABLE credentials DROP COLUMN IF EXISTS provider_id;
ALTER TABLE credentials DROP COLUMN IF EXISTS provider;
ALTER TABLE credentials ALTER COLUMN password_hash SET NOT NULL;
