-- Normalise masters.city to lowercase+trimmed so that the existing B-Tree
-- index idx_masters_city (on city) can serve equality filters without
-- wrapping the column in LOWER(TRIM(...)).
--
-- Before this migration, ListPublic applied LOWER(TRIM(m.city)) = ANY(...)
-- at query time, which prevents index usage (functional expression ≠ indexed
-- expression). After this migration the application normalises city on every
-- write (usecase/master.go UpdateMyProfile), and the filter becomes a plain
-- equality m.city = ANY(...) that hits idx_masters_city directly.
--
-- Back-fill: update all rows where LOWER(TRIM(city)) differs from city.
-- city = '' rows are untouched (LOWER(TRIM('')) = '' — no-op).
UPDATE masters
SET city = LOWER(TRIM(city))
WHERE city IS DISTINCT FROM LOWER(TRIM(city));
