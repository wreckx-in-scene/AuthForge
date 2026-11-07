CREATE SCHEMA IF NOT EXISTS tenant;

CREATE ROLE authforge_app LOGIN PASSWORD 'authforge_app' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;

CREATE TABLE tenant.accounts (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL,
    normalized_email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT accounts_email_normalized CHECK (normalized_email = lower(normalized_email))
);

CREATE TABLE tenant.organizations (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tenant.organization_members (
    organization_id UUID NOT NULL REFERENCES tenant.organizations (id),
    account_id UUID NOT NULL REFERENCES tenant.accounts (id),
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, account_id),
    CONSTRAINT organization_members_role CHECK (role IN ('owner'))
);

CREATE TABLE tenant.projects (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES tenant.organizations (id),
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug),
    UNIQUE (organization_id, id)
);

CREATE TABLE tenant.environments (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    organization_id UUID NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name),
    UNIQUE (project_id, id),
    FOREIGN KEY (organization_id, project_id) REFERENCES tenant.projects (organization_id, id)
);

CREATE TABLE tenant.api_keys (
    id UUID PRIMARY KEY,
    environment_id UUID NOT NULL,
    project_id UUID NOT NULL,
    organization_id UUID NOT NULL,
    prefix TEXT NOT NULL,
    key_hash BYTEA NOT NULL UNIQUE,
    key_class TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT api_keys_class CHECK (key_class IN ('publishable', 'secret')),
    CONSTRAINT api_keys_status CHECK (status IN ('active', 'revoked')),
    FOREIGN KEY (project_id, environment_id) REFERENCES tenant.environments (project_id, id),
    FOREIGN KEY (organization_id, project_id) REFERENCES tenant.projects (organization_id, id)
);

GRANT USAGE ON SCHEMA tenant TO authforge_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA tenant TO authforge_app;
