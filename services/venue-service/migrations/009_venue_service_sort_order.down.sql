DROP INDEX IF EXISTS idx_venue_services_venue_sort;
ALTER TABLE venue_services DROP COLUMN IF EXISTS sort_order;
