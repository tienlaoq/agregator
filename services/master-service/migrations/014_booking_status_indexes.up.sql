-- Composite indexes for ListBookingsByMaster and ListBookingsByClient.
--
-- Both methods execute:
--   WHERE {master_id|client_user_id} = $1 [AND status = $2]
--   ORDER BY date DESC, time_from DESC
--
-- The existing indexes (001_init.up.sql) do not cover status and omit
-- time_from, so a filtered+sorted query requires a partial sort on top of
-- an index scan:
--   idx_master_bookings_master  (master_id, date)          -- missing status, time_from
--   idx_master_bookings_client  (client_user_id)           -- missing date, status, time_from
--
-- New indexes include status and the full sort key so Postgres can satisfy
-- both the equality/range filter and the ORDER BY from a single index scan
-- with no extra sort step.
--
-- Column order rationale:
--   1. Equality predicate first (master_id / client_user_id).
--   2. status — when present reduces the scan range before the sort key.
--      When absent the planner uses the index for the equality lookup and
--      applies a filter on status, which is acceptable (low cardinality column,
--      few distinct values).
--   3. date DESC, time_from DESC — matches ORDER BY exactly so the index
--      delivers rows in the right order without a sort node.
--
-- The old indexes are dropped first to avoid index bloat; their access
-- patterns are fully subsumed by the new ones.

DROP INDEX IF EXISTS idx_master_bookings_master;
CREATE INDEX idx_master_bookings_master
    ON master_bookings (master_id, status, date DESC, time_from DESC);

DROP INDEX IF EXISTS idx_master_bookings_client;
CREATE INDEX idx_master_bookings_client
    ON master_bookings (client_user_id, status, date DESC, time_from DESC);
