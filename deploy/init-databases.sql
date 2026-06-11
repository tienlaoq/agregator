CREATE DATABASE auth_db;
CREATE DATABASE user_db;
CREATE DATABASE venue_db;
CREATE DATABASE booking_db;
CREATE DATABASE review_db;
CREATE DATABASE payment_db;
CREATE DATABASE master_db;
CREATE DATABASE chat_db;
CREATE DATABASE notification_db;
CREATE DATABASE support_db;
-- analytics_db добавлена позже support_db: на существующем volume создать вручную:
--   docker exec -i banya-postgres psql -U banya -d banya -c "CREATE DATABASE analytics_db;"
CREATE DATABASE analytics_db;

\c venue_db
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
