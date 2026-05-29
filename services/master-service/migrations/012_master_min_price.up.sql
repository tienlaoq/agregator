-- Denormalise the effective minimum price into masters so that ListPublic
-- price-range filters can use a plain B-Tree index instead of a per-row
-- correlated subquery.
--
-- Problem: the filter
--   COALESCE((SELECT MIN(ms.price) FROM master_services WHERE master_id = m.id
--             AND ms.price > 0), m.hourly_rate) >= $X
-- executes a full index scan of master_services for EVERY row that passes the
-- status = 'active' predicate. At 10k masters × 50 services that is ~500k
-- index lookups per catalogue page.
--
-- Solution (same pattern as chat-service/migrations/004_unread_count.up.sql):
--   1. Add a denormalised column masters.min_effective_price_kopecks.
--   2. Define a PL/pgSQL function that recomputes it for a single master.
--   3. Attach AFTER triggers on master_services (INSERT/UPDATE/DELETE) and
--      masters (UPDATE of hourly_rate) to keep the column in sync atomically.
--   4. Back-fill all existing rows.
--   5. Add a B-Tree index so the WHERE clause is a plain index range scan.
--
-- The column is NULL when a master has no services AND hourly_rate = 0
-- (profile not yet configured). ListPublic treats NULL rows as matching any
-- price filter (COALESCE(min_effective_price_kopecks, 0) in the WHERE clause),
-- consistent with the previous correlated-subquery behaviour which fell back to
-- hourly_rate = 0 and satisfied any >= 0 filter.

ALTER TABLE masters
    ADD COLUMN IF NOT EXISTS min_effective_price_kopecks BIGINT;

-- fn_recompute_master_min_price: single-master recompute, called from triggers.
-- Uses MIN(price) WHERE price > 0 so zero-priced placeholder services are
-- excluded, falling back to hourly_rate (which may itself be 0 for unconfigured
-- profiles). Matches the semantics of the old COALESCE subquery exactly.
CREATE OR REPLACE FUNCTION fn_recompute_master_min_price()
    RETURNS trigger
    LANGUAGE plpgsql AS
$$
DECLARE
    v_master_id UUID;
    v_min       BIGINT;
BEGIN
    -- Determine which master to recompute.
    -- On master_services triggers TG_TABLE_NAME = 'master_services';
    -- on masters triggers TG_TABLE_NAME = 'masters'.
    IF TG_TABLE_NAME = 'master_services' THEN
        -- Use NEW for INSERT/UPDATE; OLD for DELETE (NEW is NULL on DELETE).
        IF TG_OP = 'DELETE' THEN
            v_master_id := OLD.master_id;
        ELSE
            v_master_id := NEW.master_id;
        END IF;
    ELSE
        -- masters table trigger — hourly_rate changed.
        v_master_id := NEW.id;
    END IF;

    SELECT COALESCE(
        NULLIF((SELECT MIN(ms.price)
                FROM   master_services ms
                WHERE  ms.master_id = v_master_id
                  AND  ms.price > 0), 0),
        (SELECT hourly_rate FROM masters WHERE id = v_master_id)
    )
    INTO v_min;

    UPDATE masters
    SET    min_effective_price_kopecks = v_min
    WHERE  id = v_master_id;

    RETURN NULL; -- AFTER trigger; return value is ignored for row-level
END;
$$;

-- Trigger: service added, updated, or removed → recompute for owning master.
DROP TRIGGER IF EXISTS trg_master_min_price_services ON master_services;
CREATE TRIGGER trg_master_min_price_services
    AFTER INSERT OR UPDATE OF price OR DELETE ON master_services
    FOR EACH ROW EXECUTE FUNCTION fn_recompute_master_min_price();

-- Trigger: master hourly_rate changed → recompute (fallback price).
DROP TRIGGER IF EXISTS trg_master_min_price_hourly ON masters;
CREATE TRIGGER trg_master_min_price_hourly
    AFTER UPDATE OF hourly_rate ON masters
    FOR EACH ROW EXECUTE FUNCTION fn_recompute_master_min_price();

-- Back-fill all existing rows.
UPDATE masters m
SET min_effective_price_kopecks = (
    SELECT COALESCE(
        NULLIF((SELECT MIN(ms.price) FROM master_services ms
                WHERE ms.master_id = m.id AND ms.price > 0), 0),
        m.hourly_rate
    )
);

-- B-Tree index for range queries in ListPublic (>= min, <= max).
CREATE INDEX idx_masters_min_price ON masters (min_effective_price_kopecks)
    WHERE status = 'active';
