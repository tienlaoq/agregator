-- Enforce at most one cover photo per master at the database level.
--
-- Problem: the application logic in SetMasterCoverPhoto and AddMasterPhoto
-- maintains is_cover = true for at most one row per master, but nothing at
-- the DB level prevents a concurrent INSERT or UPDATE from producing two rows
-- with is_cover = true for the same master. A partial unique index closes
-- this gap: any second attempt to set is_cover = true for the same master_id
-- will fail with a unique violation, making the constraint race-proof.
--
-- A partial index (WHERE is_cover = true) rather than a full composite index
-- is used so that rows with is_cover = false (the common case) are not
-- included — keeping the index small and writes fast.
--
-- SetMasterCoverPhoto was rewritten to a single-statement UPDATE:
--   SET is_cover = (id = $photoID) WHERE master_id = $masterID
-- This clears all other covers and sets the new one atomically in one
-- statement, so the constraint is never violated mid-transaction.
--
-- AddMasterPhoto uses isCover = (COUNT(*) == 0) inside a transaction. If two
-- concurrent inserts both see count=0, the second will get a unique violation
-- from this index, which surfaces as a repo error and is correct behaviour
-- (the second caller should retry or accept the existing cover).

CREATE UNIQUE INDEX IF NOT EXISTS uq_master_photos_one_cover
    ON master_photos (master_id)
    WHERE is_cover = TRUE;
