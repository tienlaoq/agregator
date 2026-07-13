CREATE TABLE venue_videos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id   UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    url        TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_venue_videos_venue ON venue_videos (venue_id, sort_order);
