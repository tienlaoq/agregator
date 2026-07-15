-- Weekend hourly rate for masters. hourly_rate stays as the weekday rate;
-- price_weekend = 0 means "same as weekday" (booking falls back to hourly_rate).
ALTER TABLE masters ADD COLUMN price_weekend BIGINT NOT NULL DEFAULT 0;
