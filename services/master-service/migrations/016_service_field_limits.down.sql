ALTER TABLE master_services
    DROP CONSTRAINT IF EXISTS chk_master_service_name_length,
    DROP CONSTRAINT IF EXISTS chk_master_service_description_length,
    DROP CONSTRAINT IF EXISTS chk_master_service_duration_non_negative,
    DROP CONSTRAINT IF EXISTS chk_master_service_price_non_negative;
