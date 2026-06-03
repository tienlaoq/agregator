-- Restore provider_seller_account_id with the CHECK that 008 had installed.

BEGIN;

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS provider_seller_account_id VARCHAR(64);

ALTER TABLE payments
    ADD CONSTRAINT chk_provider_seller_account_id CHECK (
        provider_seller_account_id IS NULL
        OR (
            length(provider_seller_account_id) > 0
            AND CASE provider_name
                WHEN 'yookassa' THEN provider_seller_account_id ~ '^[0-9]{1,16}$'
                WHEN 'tbank'    THEN provider_seller_account_id ~ '^[A-Za-z0-9_-]{1,20}$'
                WHEN 'sber'     THEN provider_seller_account_id ~ '^[A-Za-z0-9_.:-]{1,64}$'
                ELSE TRUE
            END
        )
    );

COMMIT;
