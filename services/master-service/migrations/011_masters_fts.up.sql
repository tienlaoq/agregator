-- Full-text search index for the public masters catalogue (ListPublic).
--
-- Problem: the existing query uses ILIKE '%q%' across six columns plus a
-- correlated EXISTS over master_services. A leading-wildcard ILIKE never hits
-- a B-Tree index; the EXISTS re-executes a master_services scan for every row
-- that passes the WHERE status = 'active' filter. At catalogue scale this is a
-- sequential scan on both tables per request.
--
-- Solution (same pattern as venue-service/migrations/001_init.up.sql):
--   1. GIN index on a computed tsvector covering the searchable text columns.
--   2. The query replaces the ILIKE OR-chain with a single tsvector @@ tsquery
--      operator, which the GIN index can satisfy in O(log N + k) instead of O(N).
--   3. Services (name/description) are NOT included here — they are a separate
--      table. Including them via a stored generated column would require a
--      trigger; instead the query retains an EXISTS fallback for service text,
--      but only after the tsvector filter reduces the candidate set. If service
--      full-text becomes a bottleneck, add a separate GIN index on
--      master_services and join rather than EXISTS.
--
-- Language: 'russian' matches venue-service and covers the primary locale.
-- plainto_tsquery is used in the query (not to_tsquery) so raw user input is
-- safe without escaping.
--
-- pg_trgm is NOT required for this index — tsvector handles word-boundary
-- search. Trigram similarity (pg_trgm + GIN) would additionally handle
-- typos / partial-word matches and can be added in a follow-up migration when
-- Meilisearch integration is not yet ready (see TECH_DEBT.md [MASTER-FTS-MEILI]).

CREATE INDEX idx_masters_fts
    ON masters
    USING GIN (
        to_tsvector('russian',
            display_name || ' ' ||
            COALESCE(bio, '') || ' ' ||
            COALESCE(city, '') || ' ' ||
            COALESCE(array_to_string(specializations, ' '), '')
        )
    );
