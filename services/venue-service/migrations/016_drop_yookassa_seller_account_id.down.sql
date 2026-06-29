-- Reverse 016: restore the column shell (data is not recoverable).
ALTER TABLE venues
    ADD COLUMN IF NOT EXISTS yookassa_seller_account_id VARCHAR(64) NOT NULL DEFAULT '';
