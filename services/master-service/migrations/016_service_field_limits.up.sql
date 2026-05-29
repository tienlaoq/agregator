-- Enforce field length limits on master_services at the database level.
--
-- The application validates these limits in usecase.validateMasterServices
-- before calling ReplaceServices, so under normal operation no row will ever
-- violate these constraints. The CHECK constraints act as a safety net against:
--   - direct DB writes (migrations, admin scripts, future services)
--   - bugs where validation is accidentally bypassed
--
-- Limits mirror the domain constants (domain.MaxServiceName = 200,
-- domain.MaxServiceDescription = 5000). Update both together if limits change.
--
-- char_length counts Unicode code points (same as Go's len([]rune(s))),
-- consistent with the usecase validation.

ALTER TABLE master_services
    ADD CONSTRAINT chk_master_service_name_length
        CHECK (char_length(name) BETWEEN 1 AND 200),
    ADD CONSTRAINT chk_master_service_description_length
        CHECK (char_length(description) <= 5000),
    ADD CONSTRAINT chk_master_service_duration_non_negative
        CHECK (duration_min >= 0),
    ADD CONSTRAINT chk_master_service_price_non_negative
        CHECK (price >= 0);
