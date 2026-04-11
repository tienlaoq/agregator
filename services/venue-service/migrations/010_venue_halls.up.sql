CREATE TABLE venue_halls (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id    UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    price_from  BIGINT NOT NULL DEFAULT 0,
    capacity    INT NOT NULL DEFAULT 0,
    amenities   TEXT[] NOT NULL DEFAULT '{}',
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX idx_venue_halls_venue_id ON venue_halls (venue_id);

CREATE TABLE venue_hall_photos (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hall_id     UUID NOT NULL REFERENCES venue_halls(id) ON DELETE CASCADE,
    url         TEXT NOT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    is_cover    BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_venue_hall_photos_hall_id ON venue_hall_photos (hall_id);
