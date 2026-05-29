-- Composite indexes for the three sort orders used in List() and the catalog page.
-- Without these the planner falls back to seq-scan + filesort on every cache miss.
-- Note: CONCURRENTLY is omitted — migration runner wraps statements in a transaction.

CREATE INDEX IF NOT EXISTS idx_venues_status_rating
    ON venues (status, avg_rating DESC);

CREATE INDEX IF NOT EXISTS idx_venues_status_created
    ON venues (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_venues_status_price
    ON venues (status, price_from ASC NULLS LAST);
