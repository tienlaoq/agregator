ALTER TABLE masters
    ADD COLUMN IF NOT EXISTS payout_legal_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_inn VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_kpp VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_ogrn VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_ogrnip VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_bank_name VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_bik VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_settlement_account VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_correspondent_account VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS payout_verification_status VARCHAR(32) NOT NULL DEFAULT 'unverified'
        CHECK (payout_verification_status IN ('unverified', 'pending', 'verified', 'rejected'));
