-- Как мастер получает выплаты с платформы (для модерации и бэкофиса).
ALTER TABLE masters
    ADD COLUMN IF NOT EXISTS payout_legal_form VARCHAR(32) NOT NULL DEFAULT ''
        CHECK (payout_legal_form IN ('', 'ip', 'gph', 'self_employed'));
