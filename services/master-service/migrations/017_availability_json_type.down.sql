ALTER TABLE masters
    DROP CONSTRAINT IF EXISTS chk_masters_availability_json_is_object;

ALTER TABLE masters
    ALTER COLUMN availability_json TYPE TEXT
        USING availability_json::text,
    ALTER COLUMN availability_json SET DEFAULT '';
