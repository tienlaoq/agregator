ALTER TABLE venues ADD COLUMN city VARCHAR(100) DEFAULT '';
CREATE INDEX idx_venues_city ON venues (city);
