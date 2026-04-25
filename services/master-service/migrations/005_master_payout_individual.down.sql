ALTER TABLE masters DROP CONSTRAINT IF EXISTS masters_payout_legal_form_check;
UPDATE masters SET payout_legal_form = 'gph' WHERE payout_legal_form = 'individual';
ALTER TABLE masters
    ADD CONSTRAINT masters_payout_legal_form_check
        CHECK (payout_legal_form IN ('', 'ip', 'gph', 'self_employed', 'ooo'));
