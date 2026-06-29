-- Remove the ЮKassa-era seller account column. The platform no longer
-- integrates ЮKassa, and the column has been unused since the move to the
-- escrow payout model. payout_legal_form is retained — it is still part of the
-- partner payout profile and is provider-agnostic.
ALTER TABLE venues
    DROP COLUMN IF EXISTS yookassa_seller_account_id;
