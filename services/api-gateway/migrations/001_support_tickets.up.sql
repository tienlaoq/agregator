CREATE TABLE IF NOT EXISTS support_tickets (
    request_id UUID PRIMARY KEY,
    ticket_number VARCHAR(48) NOT NULL UNIQUE,
    topic TEXT NOT NULL,
    message TEXT NOT NULL,
    user_email VARCHAR(320) NOT NULL DEFAULT '',
    user_id VARCHAR(64) NOT NULL DEFAULT '',
    role VARCHAR(32) NOT NULL DEFAULT '',
    booking_id VARCHAR(128) NOT NULL DEFAULT '',
    payment_id VARCHAR(128) NOT NULL DEFAULT '',
    source_page VARCHAR(1024) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    replied_at TIMESTAMPTZ,
    replied_by VARCHAR(64) NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS support_tickets_created_at_idx ON support_tickets (created_at DESC);
