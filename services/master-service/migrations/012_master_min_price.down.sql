DROP INDEX IF EXISTS idx_masters_min_price;
DROP TRIGGER IF EXISTS trg_master_min_price_hourly ON masters;
DROP TRIGGER IF EXISTS trg_master_min_price_services ON master_services;
DROP FUNCTION IF EXISTS fn_recompute_master_min_price();
ALTER TABLE masters DROP COLUMN IF EXISTS min_effective_price_kopecks;
