-- Revert 019: restore the original per-row trigger from migration 012.

DROP TRIGGER IF EXISTS trg_master_min_price_insert  ON master_services;
DROP TRIGGER IF EXISTS trg_master_min_price_update  ON master_services;
DROP TRIGGER IF EXISTS trg_master_min_price_delete  ON master_services;

DROP FUNCTION IF EXISTS fn_min_price_after_insert();
DROP FUNCTION IF EXISTS fn_min_price_after_update();
DROP FUNCTION IF EXISTS fn_min_price_after_delete();
DROP FUNCTION IF EXISTS fn_recompute_master_min_price_ids(UUID[]);

-- Restore the original single per-row trigger (definition from migration 012).
CREATE TRIGGER trg_master_min_price_services
    AFTER INSERT OR UPDATE OF price OR DELETE ON master_services
    FOR EACH ROW EXECUTE FUNCTION fn_recompute_master_min_price();
