ALTER TABLE masters
    DROP COLUMN IF EXISTS payout_verification_status,
    DROP COLUMN IF EXISTS payout_correspondent_account,
    DROP COLUMN IF EXISTS payout_settlement_account,
    DROP COLUMN IF EXISTS payout_bik,
    DROP COLUMN IF EXISTS payout_bank_name,
    DROP COLUMN IF EXISTS payout_ogrnip,
    DROP COLUMN IF EXISTS payout_ogrn,
    DROP COLUMN IF EXISTS payout_kpp,
    DROP COLUMN IF EXISTS payout_inn,
    DROP COLUMN IF EXISTS payout_legal_name;
