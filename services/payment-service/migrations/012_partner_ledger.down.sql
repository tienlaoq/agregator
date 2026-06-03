BEGIN;

DROP INDEX IF EXISTS partner_ledger_ripe_accruals_idx;
DROP INDEX IF EXISTS partner_ledger_partner_idx;
DROP INDEX IF EXISTS partner_ledger_payout_uniq;
DROP INDEX IF EXISTS partner_ledger_reversal_uniq;
DROP INDEX IF EXISTS partner_ledger_accrual_uniq;
DROP TABLE IF EXISTS partner_ledger;

COMMIT;
