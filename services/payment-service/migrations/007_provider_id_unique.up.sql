-- Enforce one payment per provider_id.
--
-- provider_id is nullable: a row is inserted with provider_id = NULL during
-- Phase 1 (idempotency slot claimed) and filled in during Phase 3 (provider
-- call succeeded).  A plain UNIQUE constraint would reject two concurrent NULL
-- values in some DB engines, but PostgreSQL treats each NULL as distinct, so
-- a full UNIQUE INDEX would work.  We use a partial index (WHERE provider_id
-- IS NOT NULL) to be explicit about intent and to match the query pattern in
-- GetByProviderID — the planner can use this index for that lookup as well.
--
-- The old non-unique index idx_payments_provider_id is dropped: it is now
-- fully covered (and superseded) by the partial unique index below.

DROP INDEX IF EXISTS idx_payments_provider_id;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payments_provider_id
    ON payments (provider_id)
    WHERE provider_id IS NOT NULL;
