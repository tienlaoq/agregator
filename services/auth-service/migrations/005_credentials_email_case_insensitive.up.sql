-- The application normalises email to lower(trim(email)) before every lookup
-- (GetByEmail) but the original UNIQUE constraint on credentials.email is a
-- plain btree on the stored value. This allows two rows with the same address
-- in different cases (e.g. "Foo@X.com" and "foo@x.com") to coexist, which
-- causes GetByEmail to return unpredictable results and breaks uniqueness.
--
-- Fix: replace the case-sensitive constraint with a functional unique index on
-- lower(trim(email)), which matches exactly what GetByEmail queries on.
--
-- The old constraint is dropped first; the new index enforces the same
-- uniqueness guarantee plus case-folding. Existing data is assumed to be
-- already normalised by the application layer (validateRegisterInput lowercases
-- on Register; OAuthLogin normalisation is added in the same release).

ALTER TABLE credentials DROP CONSTRAINT IF EXISTS credentials_email_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_credentials_email_lower
    ON credentials (lower(trim(email)));
