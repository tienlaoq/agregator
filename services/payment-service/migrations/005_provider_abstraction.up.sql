-- HIGH-9: provider abstraction — rename yookassa-specific column, add provider_name.
--
-- Strategy: rename-with-backward-compatibility.
--   1. Add the new generic column provider_seller_account_id (nullable).
--   2. Copy existing data from yookassa_seller_account_id.
--   3. Add provider_name with DEFAULT 'yookassa' (all existing rows are ЮKassa).
--   4. Deprecate yookassa_seller_account_id by renaming it to _deprecated_*.
--      Dropping it outright would require a table rewrite; keeping it as
--      _deprecated_ makes it invisible to the new code while preserving data
--      until the next release cleanup.
--
-- The old column yookassa_seller_account_id is kept as
-- _deprecated_yookassa_seller_account_id so that any lingering readers
-- (monitoring queries, legacy migration scripts) fail loudly with a "column
-- not found" rather than silently reading NULL.

BEGIN;

-- Step 1: add new provider-agnostic seller account column.
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS provider_seller_account_id VARCHAR(255);

-- Step 2: migrate existing data.
UPDATE payments
   SET provider_seller_account_id = yookassa_seller_account_id
 WHERE yookassa_seller_account_id IS NOT NULL AND yookassa_seller_account_id <> '';

-- Step 3: add provider_name — all existing rows are ЮKassa.
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS provider_name VARCHAR(32) NOT NULL DEFAULT 'yookassa';

-- Step 4: rename old column to _deprecated_ so new code cannot accidentally
--         reference it.  The column is NOT dropped here; drop it in 006 after
--         verifying no readers remain.
ALTER TABLE payments
    RENAME COLUMN yookassa_seller_account_id TO _deprecated_yookassa_seller_account_id;

COMMIT;
