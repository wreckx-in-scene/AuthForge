CREATE TABLE session.authorization_codes (
    code_hash BYTEA PRIMARY KEY,
    environment_id UUID NOT NULL,
    project_id UUID NOT NULL,
    user_id UUID NOT NULL,
    client_id UUID NOT NULL,
    redirect_uri TEXT NOT NULL,
    code_challenge TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    CONSTRAINT authorization_codes_expiry CHECK (expires_at > created_at)
);

CREATE INDEX authorization_codes_expiry_idx ON session.authorization_codes (expires_at);

CREATE FUNCTION session.peek_authorization_code_environment(p_hash bytea)
RETURNS uuid
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = session, pg_temp
AS $$
    SELECT environment_id FROM session.authorization_codes WHERE code_hash = p_hash
$$;

REVOKE ALL ON FUNCTION session.peek_authorization_code_environment(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION session.peek_authorization_code_environment(bytea) TO authforge_app;

ALTER TABLE session.authorization_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE session.authorization_codes FORCE ROW LEVEL SECURITY;

CREATE POLICY environment_scope ON session.authorization_codes
    USING (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    )
    WITH CHECK (
        current_setting('authforge.environment_id', true) <> ''
        AND environment_id = current_setting('authforge.environment_id', true)::uuid
    );

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE session.authorization_codes TO authforge_app;
