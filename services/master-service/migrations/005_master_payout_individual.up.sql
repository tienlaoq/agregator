-- Замена формы выплат «ГПХ» на «физическое лицо» (код individual).
ALTER TABLE masters DROP CONSTRAINT IF EXISTS masters_payout_legal_form_check;
UPDATE masters SET payout_legal_form = 'individual' WHERE payout_legal_form = 'gph';
ALTER TABLE masters
    ADD CONSTRAINT masters_payout_legal_form_check
        CHECK (payout_legal_form IN ('', 'ip', 'individual', 'self_employed', 'ooo'));
