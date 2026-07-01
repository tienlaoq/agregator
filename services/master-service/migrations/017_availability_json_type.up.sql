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
-- Why the DO block / DROP DEFAULT dance:
--   The original single-statement form
--       ALTER COLUMN … TYPE JSONB USING …, ALTER COLUMN … SET DEFAULT '{}'
--   fails with "default for column \"availability_json\" cannot be cast
--   automatically to type jsonb": PostgreSQL evaluates the *existing* TEXT
--   default ('' from 001_init) against the new type BEFORE applying the new
--   default. The fix is to DROP the old default first, change the type, then
--   SET the new default. The whole conversion is wrapped in a guard so the
--   migration is idempotent — deploy/migrate.sh re-applies every *.up.sql on
--   each run, and the second run finds the column already JSONB and skips it
--   (re-running the UPDATE with `= ''` against a jsonb column would error).

DO $$
BEGIN
    IF (
        SELECT data_type
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'masters'
          AND column_name  = 'availability_json'
    ) = 'text' THEN
        -- Step 1: normalise legacy empty strings while the column is still TEXT.
        UPDATE masters
        SET availability_json = '{}'
        WHERE availability_json = '' OR availability_json IS NULL;

        -- Step 2: drop the TEXT default, change type, then set the JSONB default.
        ALTER TABLE masters ALTER COLUMN availability_json DROP DEFAULT;
        ALTER TABLE masters ALTER COLUMN availability_json TYPE JSONB
            USING availability_json::jsonb;
        ALTER TABLE masters ALTER COLUMN availability_json SET DEFAULT '{}';
    END IF;
END $$;

-- Step 3: enforce object structure (idempotent — drop-then-add).
ALTER TABLE masters
    DROP CONSTRAINT IF EXISTS chk_masters_availability_json_is_object;
ALTER TABLE masters
    ADD CONSTRAINT chk_masters_availability_json_is_object
        CHECK (jsonb_typeof(availability_json) = 'object');
