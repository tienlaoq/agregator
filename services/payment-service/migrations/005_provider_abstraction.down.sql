-- Rollback HIGH-9 provider abstraction migration.

BEGIN;

-- Restore original column name.
ALTER TABLE payments
    RENAME COLUMN _deprecated_yookassa_seller_account_id TO yookassa_seller_account_id;

-- Remove provider_name (added in up).
ALTER TABLE payments
    DROP COLUMN IF EXISTS provider_name;

-- Remove provider_seller_account_id (added in up).
ALTER TABLE payments
    DROP COLUMN IF EXISTS provider_seller_account_id;

COMMIT;
