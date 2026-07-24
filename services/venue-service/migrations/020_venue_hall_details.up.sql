-- Per-hall description and steam/heating type. Competitor parity: BaniBook shows
-- «Тип парной» and a free-text description on each hall. Empty string = not set.
ALTER TABLE venue_halls ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE venue_halls ADD COLUMN IF NOT EXISTS steam_type  TEXT NOT NULL DEFAULT '';
