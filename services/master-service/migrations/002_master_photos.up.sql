CREATE TABLE IF NOT EXISTS master_photos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_id UUID NOT NULL REFERENCES masters (id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    is_cover BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_master_photos_master ON master_photos (master_id, sort_order);
