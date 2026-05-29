-- Normalise phone numbers already stored in masters.phone to the canonical
-- 11-digit form (7XXXXXXXXXX) that the application now writes on every update.
--
-- The logic mirrors normalizeRussianMobileDigits in usecase/master.go:
--   1. Strip everything that is not a digit.
--   2. If 10 digits remain, prepend '7' (Russian mobile, no country code).
--   3. If 11 digits remain and the first is '8', replace it with '7'.
--   4. If 11 digits remain and the first is '7', keep as-is.
--   5. Anything else is left untouched (empty or non-Russian numbers that
--      passed the old, lax validation — they will fail on next profile save).
--
-- Safe to run multiple times: already-normalised rows match case 3/4 and are
-- written back unchanged.

UPDATE masters
SET phone = (
    -- Strip non-digits.
    WITH stripped AS (
        SELECT regexp_replace(phone, '[^0-9]', '', 'g') AS d
    )
    SELECT
        CASE
            WHEN length(d) = 10             THEN '7' || d
            WHEN length(d) = 11 AND left(d, 1) = '8' THEN '7' || substring(d FROM 2)
            WHEN length(d) = 11 AND left(d, 1) = '7' THEN d
            ELSE phone  -- leave unchanged; will be rejected on next update
        END
    FROM stripped
)
WHERE phone <> '';
