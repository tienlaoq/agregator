-- Append-only ledger of partner money movements.
--
-- Single source of truth for "how much do we owe partner X".
-- Balance = SUM(amount_kopecks) WHERE partner_* = X.
--
-- entry_type:
--   'accrual'  — money owed to the partner from a successful payment
--                (amount > 0; available_at set to service_at + hold).
--   'reversal' — undoes a previous accrual after a refund
--                (amount < 0; references the original entry via reverses_entry_id).
--   'payout'   — money sent to the partner via a payout
--                (amount < 0; references the payout via payout_id).
--   'adjustment' — manual correction with a written reason
--                  (any sign; used rarely; requires reason).
--
-- available_at:
--   For accruals — when the entry can be included in payout calculations.
--   For others — NULL (always considered immediately settled).
--
-- The available balance (the only number that matters for payouts) is:
--   SUM(amount_kopecks)
--   WHERE partner_* = X
--     AND (entry_type <> 'accrual' OR available_at <= now())
--
-- Uniqueness:
--   • payment_id + entry_type='accrual' is unique → exactly one accrual per
--     payment, regardless of webhook retries.
--   • payment_id + entry_type='reversal' is unique → exactly one reversal
--     per refunded payment.
--   • payout_id + entry_type='payout' is unique → exactly one payout debit.

BEGIN;

CREATE TABLE IF NOT EXISTS partner_ledger (
    id                  BIGSERIAL PRIMARY KEY,
    partner_type        VARCHAR(16) NOT NULL CHECK (partner_type IN ('venue', 'master')),
    partner_id          UUID        NOT NULL,

    entry_type          VARCHAR(16) NOT NULL CHECK (entry_type IN ('accrual', 'reversal', 'payout', 'adjustment')),
    amount_kopecks      BIGINT      NOT NULL,

    -- References (nullable depending on entry_type).
    -- RESTRICT on delete: ledger entries are immutable financial records;
    -- deleting a payment or payout that has ledger rows would leave dangling
    -- balance. Application code never deletes these — RESTRICT enforces it
    -- against accidental admin/SQL slips.
    payment_id          UUID REFERENCES payments(id) ON DELETE RESTRICT,
    payout_id           UUID, -- FK added in 013 once payouts table exists
    reverses_entry_id   BIGINT REFERENCES partner_ledger(id),

    available_at        TIMESTAMPTZ,
    reason              TEXT,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Sanity: amount sign matches entry_type. Zero is never allowed —
    -- a no-op ledger entry has no semantic meaning and is almost always a bug.
    CONSTRAINT chk_amount_sign CHECK (
        (entry_type = 'accrual'    AND amount_kopecks > 0)
        OR (entry_type = 'reversal'   AND amount_kopecks < 0)
        OR (entry_type = 'payout'     AND amount_kopecks < 0)
        OR (entry_type = 'adjustment' AND amount_kopecks <> 0)
    ),
    CONSTRAINT chk_payment_ref CHECK (
        (entry_type IN ('accrual', 'reversal') AND payment_id IS NOT NULL)
        OR (entry_type NOT IN ('accrual', 'reversal'))
    ),
    CONSTRAINT chk_payout_ref CHECK (
        (entry_type = 'payout' AND payout_id IS NOT NULL)
        OR (entry_type <> 'payout')
    ),
    CONSTRAINT chk_reversal_ref CHECK (
        (entry_type = 'reversal' AND reverses_entry_id IS NOT NULL)
        OR (entry_type <> 'reversal')
    ),
    CONSTRAINT chk_adjustment_reason CHECK (
        entry_type <> 'adjustment' OR (reason IS NOT NULL AND length(reason) > 0)
    )
);

-- Uniqueness guards against double-writes from retried webhooks / Naks.
CREATE UNIQUE INDEX IF NOT EXISTS partner_ledger_accrual_uniq
    ON partner_ledger (payment_id)
    WHERE entry_type = 'accrual';

CREATE UNIQUE INDEX IF NOT EXISTS partner_ledger_reversal_uniq
    ON partner_ledger (payment_id)
    WHERE entry_type = 'reversal';

CREATE UNIQUE INDEX IF NOT EXISTS partner_ledger_payout_uniq
    ON partner_ledger (payout_id)
    WHERE entry_type = 'payout';

-- Hot path: balance lookup + scheduler scan by partner.
CREATE INDEX IF NOT EXISTS partner_ledger_partner_idx
    ON partner_ledger (partner_type, partner_id, available_at);

-- Scheduler hot path: enumerate partners with ripe accruals and check
-- available_at via a range scan. Including available_at in the key lets
-- Postgres use an index-only scan for "available_at <= now()" filtering
-- instead of falling back to a heap recheck on every candidate row.
CREATE INDEX IF NOT EXISTS partner_ledger_ripe_accruals_idx
    ON partner_ledger (partner_type, partner_id, available_at)
    WHERE entry_type = 'accrual' AND available_at IS NOT NULL;

COMMIT;
