ALTER TABLE venue_services
  ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_venue_services_venue_sort
  ON venue_services (venue_id, sort_order);
