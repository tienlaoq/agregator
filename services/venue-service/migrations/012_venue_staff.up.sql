CREATE TABLE IF NOT EXISTS venue_staff (
    venue_id   UUID NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    role       TEXT NOT NULL CHECK (role IN ('manager', 'staff')),
    invited_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (venue_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_venue_staff_user ON venue_staff (user_id);
