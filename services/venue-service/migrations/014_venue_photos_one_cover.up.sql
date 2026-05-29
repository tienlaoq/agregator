-- Enforce at most one cover photo per venue and per hall.
-- Prevents the race between AddVenuePhoto (is_cover = first photo) and SetVenueCoverPhoto.

-- Preflight cleanup: if existing data has multiple is_cover=true rows for the same
-- venue or hall, demote all but the earliest one (lowest sort_order, then lowest id).
-- This makes the migration safe to apply against production databases.
UPDATE venue_photos p
SET    is_cover = false
WHERE  is_cover = true
  AND  id <> (
      SELECT id
      FROM   venue_photos
      WHERE  venue_id = p.venue_id
        AND  is_cover = true
      ORDER  BY sort_order ASC, id ASC
      LIMIT  1
  );

UPDATE venue_hall_photos p
SET    is_cover = false
WHERE  is_cover = true
  AND  id <> (
      SELECT id
      FROM   venue_hall_photos
      WHERE  hall_id = p.hall_id
        AND  is_cover = true
      ORDER  BY sort_order ASC, id ASC
      LIMIT  1
  );

CREATE UNIQUE INDEX IF NOT EXISTS venue_photos_one_cover
    ON venue_photos (venue_id) WHERE is_cover;

CREATE UNIQUE INDEX IF NOT EXISTS venue_hall_photos_one_cover
    ON venue_hall_photos (hall_id) WHERE is_cover;
