-- Idempotent per-booking ledger feeding the guest projection (Customer 360,
-- CRM Phase B). One row per booking_id, upserted from booking.* events. The
-- profile aggregate is derived from this table. See docs/CRM_DESIGN.md §4.

CREATE TABLE IF NOT EXISTS crm_guest_booking_facts (
    booking_id   UUID PRIMARY KEY,
    venue_id     UUID        NOT NULL,
    user_id      UUID        NOT NULL,
    status       TEXT        NOT NULL,
    status_rank  SMALLINT    NOT NULL,   -- guards against out-of-order delivery
    total_price  BIGINT      NOT NULL DEFAULT 0,
    visit_date   DATE,
    guests       INT         NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_crm_facts_guest
    ON crm_guest_booking_facts (venue_id, user_id);
