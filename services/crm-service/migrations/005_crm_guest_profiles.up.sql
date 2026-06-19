-- Guest profile projection (Customer 360, CRM Phase B). Maintained as an
-- aggregate of crm_guest_booking_facts, recomputed per (venue_id, user_id) on
-- every applied booking event. No PII — only UUIDs and counters.
-- See docs/CRM_DESIGN.md §4.

CREATE TABLE IF NOT EXISTS crm_guest_profiles (
    venue_id            UUID        NOT NULL,
    user_id             UUID        NOT NULL,
    bookings_count      INT         NOT NULL DEFAULT 0,
    visits_count        INT         NOT NULL DEFAULT 0,
    cancellations_count INT         NOT NULL DEFAULT 0,
    no_show_count       INT         NOT NULL DEFAULT 0,  -- reserved (booking.no_show TBD)
    total_spent         BIGINT      NOT NULL DEFAULT 0,
    first_visit_at      DATE,
    last_visit_at       DATE,
    last_booking_at     TIMESTAMPTZ,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (venue_id, user_id)
);

-- "recent guests" (default sort) and "top spenders" (LTV sort) lists.
CREATE INDEX IF NOT EXISTS idx_crm_profiles_last_visit
    ON crm_guest_profiles (venue_id, last_visit_at DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_crm_profiles_ltv
    ON crm_guest_profiles (venue_id, total_spent DESC);
