ALTER TABLE payments
    DROP COLUMN IF EXISTS yookassa_seller_account_id,
    DROP COLUMN IF EXISTS counterparty_id,
    DROP COLUMN IF EXISTS counterparty_type,
    DROP COLUMN IF EXISTS counterparty_net_kopecks,
    DROP COLUMN IF EXISTS platform_fee_kopecks;
