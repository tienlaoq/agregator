ALTER TABLE masters
    ADD COLUMN IF NOT EXISTS travel_exclude_zones_json TEXT NOT NULL DEFAULT '[]';

COMMENT ON COLUMN masters.travel_exclude_zones_json IS 'JSON: [{id,latitude,longitude,radius_km,label?}] — зоны, куда мастер не выезжает';
