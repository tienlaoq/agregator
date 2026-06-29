-- Reverse 014: restore the deprecated column shell (data is not recoverable)
-- and the previous provider_name default.
ALTER TABLE payments
    ALTER COLUMN provider_name SET DEFAULT 'yookassa';

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS _deprecated_yookassa_seller_account_id VARCHAR(64) NOT NULL DEFAULT '';
