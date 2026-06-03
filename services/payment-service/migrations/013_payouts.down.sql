BEGIN;

-- Drop the deferred FK before the table it references.
ALTER TABLE partner_ledger DROP CONSTRAINT IF EXISTS partner_ledger_payout_id_fkey;

DROP INDEX IF EXISTS payouts_status_idx;
DROP INDEX IF EXISTS payouts_partner_created_idx;
DROP INDEX IF EXISTS payouts_partner_active_uniq;
DROP INDEX IF EXISTS payouts_provider_payout_id_uniq;
DROP INDEX IF EXISTS payouts_idempotency_key_uniq;
DROP TABLE IF EXISTS payouts;

COMMIT;
