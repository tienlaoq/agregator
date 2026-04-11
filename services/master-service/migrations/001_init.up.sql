CREATE TABLE IF NOT EXISTS masters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE,
    slug VARCHAR(255) NOT NULL UNIQUE,
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    bio TEXT NOT NULL DEFAULT '',
    phone VARCHAR(64) NOT NULL DEFAULT '',
    city VARCHAR(128) NOT NULL DEFAULT '',
    work_format VARCHAR(32) NOT NULL DEFAULT 'both'
        CHECK (work_format IN ('venue', 'mobile', 'both')),
    travel_radius_km INT NOT NULL DEFAULT 0,
    experience_years INT NOT NULL DEFAULT 0,
    specializations TEXT[] NOT NULL DEFAULT '{}',
    hourly_rate BIGINT NOT NULL DEFAULT 0,
    availability_json TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'draft'
        CHECK (status IN (
            'draft',
            'pending_review',
            'needs_revision',
            'active',
            'rejected',
            'suspended'
        )),
    moderation_comment TEXT NOT NULL DEFAULT '',
    moderated_by UUID,
    moderated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_masters_status ON masters (status);
CREATE INDEX idx_masters_city ON masters (city) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS master_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_id UUID NOT NULL REFERENCES masters (id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    duration_min INT NOT NULL DEFAULT 60,
    price BIGINT NOT NULL DEFAULT 0,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_master_services_master ON master_services (master_id);

CREATE TABLE IF NOT EXISTS master_moderation_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_id UUID NOT NULL REFERENCES masters (id) ON DELETE CASCADE,
    old_status VARCHAR(32) NOT NULL,
    new_status VARCHAR(32) NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    changed_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_master_moderation_master ON master_moderation_history (master_id, created_at DESC);

CREATE TABLE IF NOT EXISTS master_bookings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_id UUID NOT NULL REFERENCES masters (id) ON DELETE CASCADE,
    client_user_id UUID NOT NULL,
    master_service_id UUID REFERENCES master_services (id) ON DELETE SET NULL,
    date DATE NOT NULL,
    time_from TIME NOT NULL,
    time_to TIME NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'confirmed', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_master_bookings_master ON master_bookings (master_id, date);
CREATE INDEX idx_master_bookings_client ON master_bookings (client_user_id);
