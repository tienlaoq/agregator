ALTER TABLE venues ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'pending_review';
ALTER TABLE venues ADD COLUMN moderation_comment TEXT DEFAULT '';

UPDATE venues SET status = 'active' WHERE is_active = true;
UPDATE venues SET status = 'suspended' WHERE is_active = false;

CREATE INDEX idx_venues_status ON venues (status);
