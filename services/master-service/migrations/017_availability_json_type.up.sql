-- Convert availability_json from TEXT to JSONB and add a structural constraint.
--
-- Motivation:
--   The column is always treated as a JSON object in application code (empty
--   string is normalised to '{}' on write). Using TEXT gave no DB-level
--   guarantee, so invalid JSON could be inserted via direct writes or old
--   migration scripts. JSONB:
--     * rejects syntactically invalid JSON at INSERT/UPDATE time,
--     * is indexed natively (GIN),
--     * compares/queries faster than TEXT cast.
--
-- The CHECK constraint further ensures the top-level value is always a JSON
-- object (not an array, number, string, etc.), matching the application semantics.
--
-- Migration safety:
--   Step 1 — normalise empty strings that 001_init left in existing rows.
--   Step 2 — change the column type (implicit TEXT→JSONB cast via ::jsonb).
--   Step 3 — add the structural constraint (CHECK is validated immediately,
--             but all rows are already objects after step 1).

-- Step 1: normalise legacy empty strings.
UPDATE masters
SET availability_json = '{}'
WHERE availability_json = '' OR availability_json IS NULL;

-- Step 2: convert type; DEFAULT updated to match.
ALTER TABLE masters
    ALTER COLUMN availability_json TYPE JSONB
        USING availability_json::jsonb,
    ALTER COLUMN availability_json SET DEFAULT '{}';

-- Step 3: enforce object structure.
ALTER TABLE masters
    ADD CONSTRAINT chk_masters_availability_json_is_object
        CHECK (jsonb_typeof(availability_json) = 'object');
