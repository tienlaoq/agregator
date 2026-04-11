ALTER TABLE venues
  ADD COLUMN IF NOT EXISTS social_links JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN venues.social_links IS 'Public social/messenger URLs as JSON object, e.g. {"vk":"https://..."}';
