Когда CRM понадобится собственная БД:

Создать crm_db, добавить в init-databases.sql
Backfill venue_staff и venue_crm_tasks (ETL)
Заменить venueRepo.IsVenueMember/StaffUserIDsForVenue на gRPC-вызов либо NATS read-model
Подписать crm-service на venue.deleted для cascade cleanup
Pointer cmd PG_DB изменить на crm_db в config + compose