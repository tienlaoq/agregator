DROP INDEX IF EXISTS uq_payments_provider_id;

CREATE INDEX IF NOT EXISTS idx_payments_provider_id ON payments (provider_id);
