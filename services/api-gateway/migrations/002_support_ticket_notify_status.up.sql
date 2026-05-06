ALTER TABLE support_tickets ADD COLUMN IF NOT EXISTS notify_status VARCHAR(16);
UPDATE support_tickets SET notify_status = 'ok' WHERE notify_status IS NULL;
ALTER TABLE support_tickets ALTER COLUMN notify_status SET NOT NULL;
ALTER TABLE support_tickets ADD CONSTRAINT support_tickets_notify_status_chk
    CHECK (notify_status IN ('pending', 'ok', 'failed'));
