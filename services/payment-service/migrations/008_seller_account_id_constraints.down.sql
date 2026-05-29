-- Rollback migration 008: remove format constraint and restore VARCHAR(255).

BEGIN;

ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS chk_provider_seller_account_id;

ALTER TABLE payments
    ALTER COLUMN provider_seller_account_id TYPE VARCHAR(255);

COMMIT;
