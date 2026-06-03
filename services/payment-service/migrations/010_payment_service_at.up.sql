-- service_at is the wall-clock moment the booked service begins (slot start).
-- It drives the hold period: an accrual created on payment success becomes
-- available for payout at service_at + payout_hold_hours.
--
-- Nullable because:
--   • Historical rows predate this column.
--   • Some flows (e.g. goods purchases) have no service date — they would
--     fall back to created_at + hold when the accrual is computed.
--
-- service_at is informational at the payments table; the hold calculation
-- happens once when the accrual row is written to partner_ledger.

BEGIN;

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS service_at TIMESTAMPTZ;

COMMIT;
