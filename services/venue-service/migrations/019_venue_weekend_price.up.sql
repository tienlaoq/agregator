-- Weekend hourly rate for venues and halls. price_from stays as the weekday rate;
-- price_weekend = 0 means "same as weekday" (booking falls back to price_from).
ALTER TABLE venues      ADD COLUMN price_weekend BIGINT NOT NULL DEFAULT 0;
ALTER TABLE venue_halls ADD COLUMN price_weekend BIGINT NOT NULL DEFAULT 0;
