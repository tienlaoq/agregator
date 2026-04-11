DELETE FROM reserved_slots WHERE booking_id IS NULL;

ALTER TABLE reserved_slots DROP COLUMN IF EXISTS block_note;

ALTER TABLE reserved_slots ALTER COLUMN booking_id SET NOT NULL;
