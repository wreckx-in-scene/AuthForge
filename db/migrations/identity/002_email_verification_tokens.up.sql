CREATE TABLE identity.email_verification_tokens (
    id UUID PRIMARY KEY,
    environment_id UUID NOT NULL,
    user_id UUID NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (environment_id, user_id) REFERENCES identity.users (environment_id, id)
);

CREATE INDEX email_verification_tokens_user_idx
    ON identity.email_verification_tokens (environment_id, user_id, expires_at);

ALTER TABLE identity.email_verification_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity.email_verification_tokens FORCE ROW LEVEL SECURITY;

CREATE POLICY environment_scope ON identity.email_verification_tokens
    USING (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    )
    WITH CHECK (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    );

GRANT SELECT, INSERT, UPDATE, DELETE ON identity.email_verification_tokens TO authforge_app;
