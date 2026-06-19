-- Task lifecycle (CRM Phase A): due dates, priority, completion attribution and
-- a soft-delete ('cancelled') status. See docs/CRM_DESIGN.md §3.
--
-- Idempotent (IF [NOT] EXISTS / DROP-then-ADD) so deploy/migrate.sh can re-run
-- it safely, matching the convention of 001/002.

ALTER TABLE venue_crm_tasks
    ADD COLUMN IF NOT EXISTS due_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS priority     TEXT NOT NULL DEFAULT 'normal'
                            CHECK (priority IN ('low', 'normal', 'high')),
    ADD COLUMN IF NOT EXISTS completed_by UUID,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

-- Widen the status domain to allow soft-deletion. The original inline CHECK was
-- auto-named venue_crm_tasks_status_check by PostgreSQL; drop-then-add keeps the
-- script re-runnable.
ALTER TABLE venue_crm_tasks DROP CONSTRAINT IF EXISTS venue_crm_tasks_status_check;
ALTER TABLE venue_crm_tasks
    ADD CONSTRAINT venue_crm_tasks_status_check
    CHECK (status IN ('open', 'done', 'cancelled'));

-- Drives the "my open tasks, soonest deadline first" view.
CREATE INDEX IF NOT EXISTS idx_venue_crm_tasks_assignee_open
    ON venue_crm_tasks (assignee_user_id, due_at)
    WHERE status = 'open';
