CREATE TABLE IF NOT EXISTS booking_staff_notes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id      UUID NOT NULL,
    venue_id        UUID NOT NULL,
    author_user_id  UUID NOT NULL,
    body            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_booking_staff_notes_booking ON booking_staff_notes (booking_id);
