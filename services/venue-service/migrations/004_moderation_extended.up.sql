ALTER TABLE venues ADD COLUMN IF NOT EXISTS moderated_at TIMESTAMPTZ;
ALTER TABLE venues ADD COLUMN IF NOT EXISTS moderated_by UUID;

CREATE TABLE IF NOT EXISTS venue_moderation_history (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id        UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    old_status      VARCHAR(20) NOT NULL,
    new_status      VARCHAR(20) NOT NULL,
    comment         TEXT DEFAULT '',
    changed_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_moderation_history_venue ON venue_moderation_history (venue_id);
CREATE INDEX IF NOT EXISTS idx_moderation_history_created ON venue_moderation_history (created_at DESC);
