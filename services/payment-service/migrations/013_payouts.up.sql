-- Outbound payouts to partners.
--
-- One payout = one money movement attempt for a partner against a chosen
-- payout method.  The lifecycle is:
--
--   pending    — row created, ledger debit written in the same TX, provider
--                not yet called.
--   processing — provider accepted the request, awaiting async outcome.
--   succeeded  — terminal: provider confirmed funds transferred.
--   failed     — terminal: provider rejected or final failure after retries.
--                A reversal ledger entry is written so the balance is restored.
--
-- Idempotency:
--   idempotency_key is UNIQUE.  The worker derives it deterministically from
--   (partner, run_id) so retries collapse to one provider call.
--
-- method snapshot:
--   We snapshot method_kind + last4/masked-account on the payout row so that
--   the historical payout reads correctly even after the partner deactivates
--   or replaces the payout method.

BEGIN;

CREATE TABLE IF NOT EXISTS payouts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_type        VARCHAR(16) NOT NULL CHECK (partner_type IN ('venue', 'master')),
    partner_id          UUID        NOT NULL,

    amount_kopecks      BIGINT      NOT NULL CHECK (amount_kopecks > 0),
    currency            CHAR(3)     NOT NULL DEFAULT 'RUB',

    method_id           UUID        NOT NULL REFERENCES partner_payout_methods(id),
    method_kind_snapshot VARCHAR(16) NOT NULL,
    method_display      VARCHAR(64) NOT NULL,  -- e.g. "•••• 4242" / "ACC •••6789" / "+7•••67"

    status              VARCHAR(16) NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'processing', 'succeeded', 'failed')),
    provider_name       VARCHAR(32) NOT NULL,
    provider_payout_id  VARCHAR(255),
    idempotency_key     VARCHAR(255) NOT NULL,

    failure_reason      TEXT,
    attempt_count       INT          NOT NULL DEFAULT 0,

    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    completed_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS payouts_idempotency_key_uniq
    ON payouts (idempotency_key);

-- Composite uniqueness so payout-id namespaces from different providers
-- (yookassa / tbank / sber) cannot collide. Mirrors the same convention used
-- for payments.provider_id (migration 005).
CREATE UNIQUE INDEX IF NOT EXISTS payouts_provider_payout_id_uniq
    ON payouts (provider_name, provider_payout_id)
    WHERE provider_payout_id IS NOT NULL;

-- Hard rail against double-spend: no partner may have more than one open
-- payout at a time. The scheduler reads available balance → creates a new
-- payout in a transaction; this partial unique index makes the second
-- concurrent attempt fail at commit, even if the scheduler logic glitches.
-- A failed terminal payout (status='failed') frees the partner for retry.
CREATE UNIQUE INDEX IF NOT EXISTS payouts_partner_active_uniq
    ON payouts (partner_type, partner_id)
    WHERE status IN ('pending', 'processing');

CREATE INDEX IF NOT EXISTS payouts_partner_created_idx
    ON payouts (partner_type, partner_id, created_at DESC);

CREATE INDEX IF NOT EXISTS payouts_status_idx
    ON payouts (status)
    WHERE status IN ('pending', 'processing');

-- Deferred FK from partner_ledger.payout_id → payouts.id.
-- Declared here (not in 012) because payouts is created after partner_ledger.
-- RESTRICT for the same reason as payment_id: ledger rows are immutable
-- financial records; deleting a referenced payout would orphan balance.
ALTER TABLE partner_ledger
    ADD CONSTRAINT partner_ledger_payout_id_fkey
    FOREIGN KEY (payout_id) REFERENCES payouts(id) ON DELETE RESTRICT;

COMMIT;
