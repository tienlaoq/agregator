-- Payout profile for ЮKassa split (seller account_id) and legal form (ИП / ООО / самозанятость / ГПХ).
ALTER TABLE venues
    ADD COLUMN IF NOT EXISTS payout_legal_form VARCHAR(32) NOT NULL DEFAULT ''
        CHECK (payout_legal_form IN ('', 'ip', 'ooo', 'self_employed', 'gph')),
    ADD COLUMN IF NOT EXISTS yookassa_seller_account_id VARCHAR(64) NOT NULL DEFAULT '';
