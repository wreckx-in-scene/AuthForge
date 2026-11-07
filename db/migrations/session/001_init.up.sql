CREATE SCHEMA IF NOT EXISTS session;

CREATE TABLE session.sessions (
    id UUID PRIMARY KEY,
    environment_id UUID NOT NULL,
    project_id UUID NOT NULL,
    user_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    UNIQUE (environment_id, id),
    CONSTRAINT sessions_status CHECK (status IN ('active', 'revoked'))
);

CREATE TABLE session.refresh_tokens (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL,
    environment_id UUID NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    parent_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    FOREIGN KEY (environment_id, session_id) REFERENCES session.sessions (environment_id, id),
    FOREIGN KEY (parent_id) REFERENCES session.refresh_tokens (id)
);

-- The refresh token is the credential, so the lookup cannot know the environment
-- in advance. This function only returns the environment for a hash the caller
-- already has. Later statements in the transaction run under row-level security.
CREATE FUNCTION session.peek_refresh_environment(p_hash bytea)
RETURNS uuid
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = session, pg_temp
AS $$
    SELECT environment_id FROM session.refresh_tokens WHERE token_hash = p_hash
$$;

REVOKE ALL ON FUNCTION session.peek_refresh_environment(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION session.peek_refresh_environment(bytea) TO authforge_app;

ALTER TABLE session.sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE session.sessions FORCE ROW LEVEL SECURITY;
ALTER TABLE session.refresh_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE session.refresh_tokens FORCE ROW LEVEL SECURITY;

CREATE POLICY environment_scope ON session.sessions
    USING (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    )
    WITH CHECK (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    );

CREATE POLICY environment_scope ON session.refresh_tokens
    USING (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    )
    WITH CHECK (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    );

GRANT USAGE ON SCHEMA session TO authforge_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA session TO authforge_app;
