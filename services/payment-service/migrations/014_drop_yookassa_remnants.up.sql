-- Final cleanup of the ЮKassa-era schema. The platform no longer integrates
-- ЮKassa; payments run through the provider selected by PAYMENT_PROVIDER
-- (default 'mock' until a bank acquiring gateway is wired up).
--
--   1. Drop the long-deprecated seller account column. Migration 005 renamed it
--      to _deprecated_* and noted it should be dropped "in a later migration
--      after verifying no readers remain" — no code reads it; this is that drop.
--      Both names are dropped: depending on whether 005's non-idempotent RENAME
--      took effect on a given DB, the column may be present under either name.
--   2. Repoint provider_name's default away from the removed 'yookassa' value.
--      New rows always receive an explicit provider_name from the repository, so
--      the default only matters for ad-hoc inserts; 'mock' matches the new
--      out-of-the-box provider.
ALTER TABLE payments
    DROP COLUMN IF EXISTS _deprecated_yookassa_seller_account_id,
    DROP COLUMN IF EXISTS yookassa_seller_account_id;

ALTER TABLE payments
    ALTER COLUMN provider_name SET DEFAULT 'mock';
