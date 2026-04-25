-- Split metadata: platform fee (kopecks), counterparty net, ЮKassa seller shop account_id for transfers[].
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS platform_fee_kopecks BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS counterparty_net_kopecks BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS counterparty_type VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS counterparty_id VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS yookassa_seller_account_id VARCHAR(64) NOT NULL DEFAULT '';
