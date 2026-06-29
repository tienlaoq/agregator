-- Remove the ЮKassa-era seller account column on masters. Unused since the move
-- to the escrow payout model; the platform no longer integrates ЮKassa.
ALTER TABLE masters
    DROP COLUMN IF EXISTS yookassa_seller_account_id;
