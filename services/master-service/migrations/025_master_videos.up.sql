CREATE TABLE IF NOT EXISTS master_videos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_id  UUID NOT NULL REFERENCES masters (id) ON DELETE CASCADE,
    url        TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_master_videos_master ON master_videos (master_id, sort_order);
