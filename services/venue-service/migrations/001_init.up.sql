CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE venues (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    UUID NOT NULL,
    slug        VARCHAR(255) UNIQUE NOT NULL,
    name        VARCHAR(255) NOT NULL,
    type        VARCHAR(50) NOT NULL CHECK (type IN ('banya','sauna','hammam','fitobochka')),
    description TEXT,
    address     TEXT NOT NULL,
    location    GEOGRAPHY(Point, 4326) NOT NULL,
    price_from  BIGINT DEFAULT 0,
    capacity    INT DEFAULT 0,
    amenities   TEXT[] DEFAULT '{}',
    working_hours JSONB DEFAULT '{}',
    phone       VARCHAR(20),
    avg_rating  FLOAT DEFAULT 0,
    review_count INT DEFAULT 0,
    is_active   BOOLEAN DEFAULT true,
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE venue_services (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id     UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    name         VARCHAR(255) NOT NULL,
    duration_min INT,
    price        BIGINT DEFAULT 0,
    description  TEXT
);

CREATE TABLE venue_photos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id   UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    url        TEXT NOT NULL,
    sort_order INT DEFAULT 0,
    is_cover   BOOLEAN DEFAULT false
);

CREATE TABLE reserved_slots (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id   UUID NOT NULL,
    booking_id UUID NOT NULL,
    date       DATE NOT NULL,
    time_from  TIME NOT NULL,
    time_to    TIME NOT NULL,
    UNIQUE (venue_id, date, time_from)
);

CREATE INDEX idx_venues_location ON venues USING GIST (location);
CREATE INDEX idx_venues_type ON venues (type);
CREATE INDEX idx_venues_slug ON venues (slug);
CREATE INDEX idx_venues_fts ON venues USING GIN (to_tsvector('russian', name || ' ' || COALESCE(description, '') || ' ' || address));
