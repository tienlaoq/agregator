BEGIN;

DROP INDEX IF EXISTS partner_payout_methods_partner_idx;
DROP INDEX IF EXISTS partner_payout_methods_active_uniq;
DROP TABLE IF EXISTS partner_payout_methods;

COMMIT;
