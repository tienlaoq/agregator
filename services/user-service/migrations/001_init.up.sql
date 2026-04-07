CREATE TABLE IF NOT EXISTS users (
    id         UUID PRIMARY KEY,
    email      VARCHAR(255) NOT NULL UNIQUE,
    phone      VARCHAR(20),
    name       VARCHAR(255) NOT NULL,
    role       VARCHAR(50) NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'venue_owner', 'master', 'admin')),
    avatar_url TEXT,
    bio        TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
