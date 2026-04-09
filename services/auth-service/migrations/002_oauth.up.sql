ALTER TABLE credentials ALTER COLUMN password_hash DROP NOT NULL;

ALTER TABLE credentials ADD COLUMN provider VARCHAR(50);
ALTER TABLE credentials ADD COLUMN provider_id VARCHAR(255);

CREATE UNIQUE INDEX IF NOT EXISTS idx_credentials_provider ON credentials (provider, provider_id) WHERE provider IS NOT NULL;
