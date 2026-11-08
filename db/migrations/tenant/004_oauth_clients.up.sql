CREATE TABLE tenant.oauth_clients (
    id UUID PRIMARY KEY,
    environment_id UUID NOT NULL,
    project_id UUID NOT NULL,
    organization_id UUID NOT NULL,
    name TEXT NOT NULL,
    redirect_uris TEXT[] NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT oauth_clients_status CHECK (status IN ('active', 'disabled')),
    CONSTRAINT oauth_clients_redirect_uris CHECK (cardinality(redirect_uris) BETWEEN 1 AND 10),
    FOREIGN KEY (project_id, environment_id) REFERENCES tenant.environments (project_id, id),
    FOREIGN KEY (organization_id, project_id) REFERENCES tenant.projects (organization_id, id)
);

CREATE INDEX oauth_clients_environment_idx ON tenant.oauth_clients (environment_id, created_at, id);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE tenant.oauth_clients TO authforge_app;
