DROP TABLE IF EXISTS email_verification_tokens;
ALTER TABLE credentials DROP COLUMN IF EXISTS email_verified;
