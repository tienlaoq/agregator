DROP INDEX IF EXISTS idx_master_bookings_master;
CREATE INDEX idx_master_bookings_master ON master_bookings (master_id, date);

DROP INDEX IF EXISTS idx_master_bookings_client;
CREATE INDEX idx_master_bookings_client ON master_bookings (client_user_id);
