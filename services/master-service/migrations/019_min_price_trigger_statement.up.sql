-- Migration 019: replace per-row min_price triggers with statement-level triggers
-- using transition tables (REFERENCING NEW/OLD TABLE).
--
-- Problem (migration 012): trg_master_min_price_services fires FOR EACH ROW,
-- so ReplaceServices (DELETE all + INSERT N) triggers N+1 UPDATE masters
-- statements inside a single transaction. At N=50 that is 51 redundant writes
-- to the masters row — a hot spot that will cause lock contention as the
-- catalogue grows.
--
-- Solution: statement-level triggers with transition tables, available since
-- PostgreSQL 10. Each DML statement produces exactly ONE trigger invocation
-- that sees ALL affected rows at once. A single UPDATE masters ... WHERE id =
-- ANY(...) replaces the N+1 row-level updates.
--
-- Transition table availability by statement type:
--   INSERT  → NEW TABLE only  (inserted_rows)
--   UPDATE  → NEW TABLE + OLD TABLE
--   DELETE  → OLD TABLE only  (deleted_rows)
-- Statement-level triggers do not expose NEW/OLD row variables, only tables.
--
-- The masters hourly_rate trigger stays FOR EACH ROW — it fires at most once
-- per profile update and the row-level approach is simpler there.

-- ── 1. Replace the per-row services trigger with three statement-level triggers.
--      (One per DML type so each can declare the correct transition table.)

DROP TRIGGER IF EXISTS trg_master_min_price_services ON master_services;

-- fn_recompute_master_min_price_ids: recompute for a set of master_ids passed
-- as an array. Replaces the single-master variant used by the row-level trigger.
-- A single UPDATE with WHERE id = ANY($ids) is cheaper than N individual UPDATEs
-- and avoids repeated index lookups on masters.
CREATE OR REPLACE FUNCTION fn_recompute_master_min_price_ids(p_master_ids UUID[])
    RETURNS void
    LANGUAGE plpgsql AS
$$
BEGIN
    UPDATE masters m
    SET    min_effective_price_kopecks = sub.v_min
    FROM (
        SELECT
            m2.id,
            COALESCE(
                NULLIF(MIN(ms.price) FILTER (WHERE ms.price > 0), 0),
                m2.hourly_rate
            ) AS v_min
        FROM   masters m2
        LEFT   JOIN master_services ms ON ms.master_id = m2.id
        WHERE  m2.id = ANY(p_master_ids)
        GROUP  BY m2.id, m2.hourly_rate
    ) sub
    WHERE m.id = sub.id;
END;
$$;

-- INSERT trigger: collect distinct master_ids from newly inserted rows.
CREATE OR REPLACE FUNCTION fn_min_price_after_insert()
    RETURNS trigger
    LANGUAGE plpgsql AS
$$
DECLARE
    v_ids UUID[];
BEGIN
    SELECT ARRAY_AGG(DISTINCT master_id)
    INTO   v_ids
    FROM   inserted_rows;   -- transition table alias declared below

    IF v_ids IS NOT NULL THEN
        PERFORM fn_recompute_master_min_price_ids(v_ids);
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER trg_master_min_price_insert
    AFTER INSERT ON master_services
    REFERENCING NEW TABLE AS inserted_rows
    FOR EACH STATEMENT EXECUTE FUNCTION fn_min_price_after_insert();

-- UPDATE trigger: union of old and new master_ids covers moves between masters
-- (edge case, but correct).
CREATE OR REPLACE FUNCTION fn_min_price_after_update()
    RETURNS trigger
    LANGUAGE plpgsql AS
$$
DECLARE
    v_ids UUID[];
BEGIN
    SELECT ARRAY_AGG(DISTINCT master_id)
    INTO   v_ids
    FROM (
        SELECT master_id FROM updated_new_rows
        UNION
        SELECT master_id FROM updated_old_rows
    ) combined;

    IF v_ids IS NOT NULL THEN
        PERFORM fn_recompute_master_min_price_ids(v_ids);
    END IF;
    RETURN NULL;
END;
$$;

-- NOTE: no `OF price` column list here. PostgreSQL forbids combining a column
-- list with transition tables ("transition tables cannot be specified for
-- triggers with column lists"), which made the original statement fail outright.
-- Firing on every UPDATE is correct: fn_min_price_after_update recomputes the
-- derived min price, which is a no-op when price did not actually change. In
-- practice ReplaceServices rewrites rows via DELETE+INSERT, so this trigger
-- rarely fires at all.
CREATE OR REPLACE TRIGGER trg_master_min_price_update
    AFTER UPDATE ON master_services
    REFERENCING NEW TABLE AS updated_new_rows OLD TABLE AS updated_old_rows
    FOR EACH STATEMENT EXECUTE FUNCTION fn_min_price_after_update();

-- DELETE trigger: collect master_ids from deleted rows.
CREATE OR REPLACE FUNCTION fn_min_price_after_delete()
    RETURNS trigger
    LANGUAGE plpgsql AS
$$
DECLARE
    v_ids UUID[];
BEGIN
    SELECT ARRAY_AGG(DISTINCT master_id)
    INTO   v_ids
    FROM   deleted_rows;    -- transition table alias declared below

    IF v_ids IS NOT NULL THEN
        PERFORM fn_recompute_master_min_price_ids(v_ids);
    END IF;
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER trg_master_min_price_delete
    AFTER DELETE ON master_services
    REFERENCING OLD TABLE AS deleted_rows
    FOR EACH STATEMENT EXECUTE FUNCTION fn_min_price_after_delete();

-- ── 2. Keep fn_recompute_master_min_price for the masters hourly_rate trigger —
--      it fires at most once per profile update; row-level is fine there.
--      No changes to trg_master_min_price_hourly.
