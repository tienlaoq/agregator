-- The application normalises email to lower(trim(email)) before every lookup
-- (GetByEmail) and on insert (Create) but the original UNIQUE constraint on
-- users.email is a plain btree on the stored value. This allows two rows with
-- the same address in different cases (e.g. "Foo@X.com" and "foo@x.com") to
-- coexist, which causes GetUserByEmail to return unpredictable results and
-- breaks uniqueness.
--
-- Fix: replace the case-sensitive constraint with a functional unique index on
-- lower(trim(email)), which matches exactly what GetByEmail queries on.
--
-- The old constraint is dropped first; the new index enforces the same
-- uniqueness guarantee plus case-folding. Existing data is assumed to be
-- already normalised by the application layer; if duplicates differing only
-- in case exist, the CREATE INDEX will fail and the data must be cleaned up
-- manually before re-running the migration.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower
    ON users (lower(trim(email)));
