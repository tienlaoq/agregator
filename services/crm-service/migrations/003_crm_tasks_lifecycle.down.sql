DROP INDEX IF EXISTS idx_venue_crm_tasks_assignee_open;

ALTER TABLE venue_crm_tasks DROP CONSTRAINT IF EXISTS venue_crm_tasks_status_check;
ALTER TABLE venue_crm_tasks
    ADD CONSTRAINT venue_crm_tasks_status_check CHECK (status IN ('open', 'done'));

ALTER TABLE venue_crm_tasks
    DROP COLUMN IF EXISTS due_at,
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS completed_by,
    DROP COLUMN IF EXISTS completed_at;
