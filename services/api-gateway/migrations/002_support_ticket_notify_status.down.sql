ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS support_tickets_notify_status_chk;
ALTER TABLE support_tickets DROP COLUMN IF EXISTS notify_status;
