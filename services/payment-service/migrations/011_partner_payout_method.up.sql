-- Partner payout method: where to send the partner's money.
--
-- One active method per (partner_type, partner_id) at a time.  History rows
-- (deactivated_at IS NOT NULL) are kept for audit — payouts that have already
-- referenced an old method must remain joinable.
--
-- PCI / security:
--   We never store full PANs.  For card payouts we keep only the last 4 digits
--   and a provider-issued token (provider_token) that the provider exchanges
--   back into routable card data at payout time.  For bank-account payouts
--   the BIC + account number is stored as-is (not card data, low sensitivity
--   under PCI but still sensitive under 152-ФЗ).
--
-- method_kind values:
--   'card'         — provider_token + last4 set
--   'bank_account' — bank_account_bic + bank_account_number + recipient_name set
--   'sbp'          — sbp_phone (E.164) set, optional bank_id
--
-- A CHECK enforces field presence per kind so wrong-shape rows can't reach
-- the payout worker.

BEGIN;

CREATE TABLE IF NOT EXISTS partner_payout_methods (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_type        VARCHAR(16) NOT NULL CHECK (partner_type IN ('venue', 'master')),
    partner_id          UUID        NOT NULL,

    method_kind         VARCHAR(16) NOT NULL CHECK (method_kind IN ('card', 'bank_account', 'sbp')),

    -- provider_name identifies which payout provider issued the tokens / will
    -- be used to deliver the payout. Tokens are provider-scoped: a YooKassa
    -- provider_token is not interchangeable with a T-Bank one. NULL for
    -- bank_account where any provider can route via БИК+счёт.
    provider_name       VARCHAR(32),

    -- Card fields
    card_last4          CHAR(4),
    card_brand          VARCHAR(16),
    provider_token      TEXT,           -- opaque, provider-issued; replay-safe

    -- Bank-account fields
    bank_account_bic    VARCHAR(9),     -- РФ БИК = 9 цифр
    bank_account_number VARCHAR(20),    -- 20 digits
    bank_name           VARCHAR(255),
    recipient_inn       VARCHAR(12),    -- ИНН ИП = 12, ЮЛ = 10
    recipient_kpp       VARCHAR(9),     -- КПП ЮЛ only, ИП = NULL
    recipient_name      VARCHAR(255),   -- ФИО / название организации

    -- SBP fields
    sbp_phone           VARCHAR(16),    -- E.164 +7XXXXXXXXXX
    sbp_bank_id         VARCHAR(32),    -- НСПК bank ID, optional

    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deactivated_at      TIMESTAMPTZ,

    CONSTRAINT chk_card_fields CHECK (
        method_kind <> 'card'
        OR (
            card_last4 ~ '^[0-9]{4}$'
            AND provider_token IS NOT NULL AND length(provider_token) > 0
            AND provider_name IS NOT NULL AND length(provider_name) > 0
        )
    ),
    CONSTRAINT chk_bank_fields CHECK (
        method_kind <> 'bank_account'
        OR (
            bank_account_bic ~ '^[0-9]{9}$'
            AND bank_account_number ~ '^[0-9]{20}$'
            AND recipient_name IS NOT NULL AND length(recipient_name) > 0
            AND (recipient_inn IS NULL OR recipient_inn ~ '^[0-9]{10}$' OR recipient_inn ~ '^[0-9]{12}$')
            AND (recipient_kpp IS NULL OR recipient_kpp ~ '^[0-9]{9}$')
        )
    ),
    CONSTRAINT chk_sbp_fields CHECK (
        method_kind <> 'sbp'
        OR (sbp_phone ~ '^\+7[0-9]{10}$')
    )
);

-- Exactly one active method per partner.  Old methods stay (deactivated_at SET)
-- so historical payouts remain joinable.
CREATE UNIQUE INDEX IF NOT EXISTS partner_payout_methods_active_uniq
    ON partner_payout_methods (partner_type, partner_id)
    WHERE deactivated_at IS NULL;

CREATE INDEX IF NOT EXISTS partner_payout_methods_partner_idx
    ON partner_payout_methods (partner_type, partner_id);

COMMIT;
