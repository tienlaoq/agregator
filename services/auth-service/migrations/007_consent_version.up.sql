-- Records which version of the 152-ФЗ consent text the user accepted at
-- registration. The WHEN is already covered by credentials.created_at; only the
-- WHAT (text version) is new. Nullable on purpose: existing rows and OAuth
-- sign-ups (no consent checkbox shown) stay NULL rather than claiming a consent
-- that was never ticked.
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS consent_version TEXT;
