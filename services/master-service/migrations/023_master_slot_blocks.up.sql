-- Master slot blocks: intervals when a steam-master does not accept bookings —
-- vacation, a busy day, or a one-off break. Analogous to the venue side's
-- manual slot blocks, but dedicated to the solo master (no hall dimension).
--
-- Granularity: a block is a single DATE plus an OPTIONAL time interval.
--   - time_from IS NULL AND time_to IS NULL  → whole day blocked
--   - both set (time_to > time_from)         → that interval on that date
-- The CHECK enforces this invariant at the DB level as a safety net against
-- direct writes / validation bypass.
--
-- ON DELETE CASCADE ties blocks to the master's lifecycle. char_length counts
-- Unicode code points, consistent with Go's len([]rune(s)) — mirrors
-- domain.MaxSlotBlockNote (200).

CREATE TABLE IF NOT EXISTS master_slot_blocks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_id UUID NOT NULL REFERENCES masters (id) ON DELETE CASCADE,
    date DATE NOT NULL,
    time_from TIME,
    time_to TIME,
    note VARCHAR(200) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_master_slot_block_interval
        CHECK (
            (time_from IS NULL AND time_to IS NULL)
            OR (time_from IS NOT NULL AND time_to IS NOT NULL AND time_to > time_from)
        ),
    CONSTRAINT chk_master_slot_block_note_length
        CHECK (char_length(note) <= 200)
);

CREATE INDEX IF NOT EXISTS idx_master_slot_blocks_master_date
    ON master_slot_blocks (master_id, date);
