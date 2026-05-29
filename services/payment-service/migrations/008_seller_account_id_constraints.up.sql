-- Tighten provider_seller_account_id: shrink from VARCHAR(255) to VARCHAR(64)
-- and add per-provider format CHECKs so the DB rejects garbage at the boundary.
--
-- Known formats (all have significant headroom inside VARCHAR(64)):
--   yookassa — numeric string, ≤16 digits  (e.g. "1234567")
--   tbank    — alphanumeric terminalKey, ≤20 chars (e.g. "TinkoffBankTest_TERM")
--   sber     — alphanumeric merchantLogin, docs unclear; 64 is conservative
--
-- CHECK logic:
--   NULL is always allowed (simple charges have no seller account).
--   When NOT NULL the value must be non-empty and match the provider's format.
--   provider_name is already NOT NULL with DEFAULT 'yookassa' (migration 005).
--
-- Why a single constraint rather than one per provider:
--   A CASE-style CHECK is evaluated atomically and reads clearly.  Adding a new
--   provider requires only an ALTER TABLE … DROP CONSTRAINT / ADD CONSTRAINT pair
--   in the next migration — no schema rebuilds.

BEGIN;

-- Step 1: shrink the column.  PostgreSQL can shorten VARCHAR without a rewrite
-- when no existing value exceeds the new limit (all real account IDs are short).
ALTER TABLE payments
    ALTER COLUMN provider_seller_account_id TYPE VARCHAR(64);

-- Step 2: add per-provider format constraint.
--   • yookassa: digits only, 1–16 chars
--   • tbank:    alphanumeric + hyphen/underscore, 1–20 chars
--   • sber:     alphanumeric + hyphen/underscore/dot, 1–64 chars
--   • any other future provider: no format restriction yet (fails open)
ALTER TABLE payments
    ADD CONSTRAINT chk_provider_seller_account_id CHECK (
        provider_seller_account_id IS NULL
        OR (
            length(provider_seller_account_id) > 0
            AND CASE provider_name
                WHEN 'yookassa' THEN provider_seller_account_id ~ '^[0-9]{1,16}$'
                WHEN 'tbank'    THEN provider_seller_account_id ~ '^[A-Za-z0-9_-]{1,20}$'
                WHEN 'sber'     THEN provider_seller_account_id ~ '^[A-Za-z0-9_.:-]{1,64}$'
                ELSE TRUE
            END
        )
    );

COMMIT;
