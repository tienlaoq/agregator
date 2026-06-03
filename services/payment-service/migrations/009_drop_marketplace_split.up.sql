-- Drop marketplace-split fields: with escrow model we no longer split at the
-- provider level (no transfers[], no per-partner seller account).  All charges
-- land on a single platform account; payouts to partners are issued separately
-- via the new payout pipeline.
--
-- Kept on purpose:
--   counterparty_type/id   — partner identity; needed for ledger attribution.
--   counterparty_net_kopecks — partner gross after platform fee, frozen at
--                              payment time; the source of truth for the
--                              accrual amount written to partner_ledger.
--   platform_fee_kopecks   — same, for reporting/reconciliation.
--   provider_name/id       — provider-agnostic; still relevant for the charge.

BEGIN;

ALTER TABLE payments DROP CONSTRAINT IF EXISTS chk_provider_seller_account_id;
ALTER TABLE payments DROP COLUMN IF EXISTS provider_seller_account_id;

COMMIT;
