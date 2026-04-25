CREATE TABLE IF NOT EXISTS venue_crm_tasks (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id           UUID NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    booking_id         UUID,
    title              TEXT NOT NULL,
    body               TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'done')),
    assignee_user_id   UUID,
    created_by         UUID NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_venue_crm_tasks_venue_status ON venue_crm_tasks (venue_id, status);
