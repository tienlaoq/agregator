-- Master credentials: certificates and awards a steam-master can showcase on
-- their profile. A single table with a `kind` discriminator covers both
-- сертификаты ('certificate') and награды ('award') so the catalogue can render
-- them as separate sections from one source of truth.
--
-- Replace-on-update semantics mirror master_services: the whole list is deleted
-- and re-inserted inside one transaction (see repository.ReplaceCredentials).
-- ON DELETE CASCADE keeps rows tied to the master's lifecycle.
--
-- CHECK constraints mirror the domain constants
-- (domain.MaxCredentialTitle = 200, domain.MaxCredentialIssuer = 200) and act as
-- a DB-level safety net against direct writes / accidental validation bypass.
-- char_length counts Unicode code points, consistent with Go's len([]rune(s)).

CREATE TABLE IF NOT EXISTS master_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_id UUID NOT NULL REFERENCES masters (id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    title VARCHAR(200) NOT NULL,
    issuer VARCHAR(200) NOT NULL DEFAULT '',
    -- year of issue; 0 means "not specified" (the field is optional in the UI).
    year INT NOT NULL DEFAULT 0,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_master_credential_kind
        CHECK (kind IN ('certificate', 'award')),
    CONSTRAINT chk_master_credential_title_length
        CHECK (char_length(title) BETWEEN 1 AND 200),
    CONSTRAINT chk_master_credential_issuer_length
        CHECK (char_length(issuer) <= 200),
    CONSTRAINT chk_master_credential_year
        CHECK (year = 0 OR year BETWEEN 1900 AND 2100)
);

CREATE INDEX idx_master_credentials_master ON master_credentials (master_id);
