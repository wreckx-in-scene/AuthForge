CREATE SCHEMA IF NOT EXISTS identity;

CREATE TABLE identity.users (
    id UUID PRIMARY KEY,
    environment_id UUID NOT NULL,
    project_id UUID NOT NULL,
    email TEXT NOT NULL,
    normalized_email TEXT NOT NULL,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (environment_id, normalized_email),
    UNIQUE (environment_id, id),
    CONSTRAINT users_status CHECK (status IN ('active', 'disabled')),
    CONSTRAINT users_email_normalized CHECK (normalized_email = lower(normalized_email))
);

CREATE TABLE identity.credentials (
    environment_id UUID NOT NULL,
    user_id UUID NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (environment_id, user_id),
    FOREIGN KEY (environment_id, user_id) REFERENCES identity.users (environment_id, id)
);

ALTER TABLE identity.users ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity.users FORCE ROW LEVEL SECURITY;
ALTER TABLE identity.credentials ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity.credentials FORCE ROW LEVEL SECURITY;

CREATE POLICY environment_scope ON identity.users
    USING (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    )
    WITH CHECK (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    );

CREATE POLICY environment_scope ON identity.credentials
    USING (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    )
    WITH CHECK (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    );

GRANT USAGE ON SCHEMA identity TO authforge_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA identity TO authforge_app;
