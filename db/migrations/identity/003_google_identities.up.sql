CREATE TABLE identity.external_identities (
    environment_id UUID NOT NULL,
    provider TEXT NOT NULL,
    provider_subject TEXT NOT NULL,
    user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (environment_id, provider, provider_subject),
    UNIQUE (environment_id, provider, user_id),
    CONSTRAINT external_identities_provider CHECK (provider IN ('google')),
    FOREIGN KEY (environment_id, user_id) REFERENCES identity.users (environment_id, id)
);

ALTER TABLE identity.external_identities ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity.external_identities FORCE ROW LEVEL SECURITY;

CREATE POLICY environment_scope ON identity.external_identities
    USING (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    )
    WITH CHECK (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    );

GRANT SELECT, INSERT, UPDATE, DELETE ON identity.external_identities TO authforge_app;

CREATE TABLE identity.google_auth_nonces (
    id UUID PRIMARY KEY,
    environment_id UUID NOT NULL,
    nonce_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT google_auth_nonces_environment_id_id_unique UNIQUE (environment_id, id)
);

CREATE INDEX google_auth_nonces_environment_expiry_idx
    ON identity.google_auth_nonces (environment_id, expires_at);

ALTER TABLE identity.google_auth_nonces ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity.google_auth_nonces FORCE ROW LEVEL SECURITY;

CREATE POLICY environment_scope ON identity.google_auth_nonces
    USING (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    )
    WITH CHECK (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    );

GRANT SELECT, INSERT, UPDATE, DELETE ON identity.google_auth_nonces TO authforge_app;
