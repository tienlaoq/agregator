-- Soft delete: deleted_at marks an anonymised, deactivated account. The row is
-- retained so foreign keys from other services (bookings, reviews, payments)
-- stay valid; the account-deletion flow anonymises email/name/phone and stamps
-- deleted_at. Partial index keeps "active users" scans cheap.
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_users_active
    ON users (id) WHERE deleted_at IS NULL;
