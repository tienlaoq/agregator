-- Email verification gate for partner accounts (master / venue_owner).
-- New column starts false for accounts created after this migration; existing
-- rows are backfilled to true so current users are never locked out of partner
-- actions they could already perform before the gate existed.
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT false;

-- Backfill: everyone who already exists predates the gate, so treat them as verified.
UPDATE credentials SET email_verified = true WHERE email_verified = false;

-- Token table mirrors password_reset_tokens (003): hashed single-use token,
-- TTL via expires_at, used_at marks consumption, cascade on credential delete.
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES credentials(user_id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_user_id ON email_verification_tokens (user_id);
