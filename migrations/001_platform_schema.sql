create extension if not exists "pgcrypto";

CREATE TABLE IF NOT EXISTS accounts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255) NOT NULL , 
  name VARCHAR(255) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects(
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  publishable_key VARCHAR(64) UNIQUE NOT NULL,
  secret_key_hash VARCHAR(64) UNIQUE NOT NULL,
  jwt_secret VARCHAR(255) NOT NULL,
  google_client_id VARCHAR(255),
  google_client_secret VARCHAR(255),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_projects_owner on projects(owner_id);
CREATE INDEX IF NOT EXISTS idx_projects_publishable_key ON projects(publishable_key);
CREATE INDEX IF NOT EXISTS idx_secret_key_hash ON projects(secret_key_hash);

CREATE TABLE IF NOT EXISTS users(
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  email VARCHAR(255) NOT NULL,
  password_hash VARCHAR(255),
  NAME VARCHAR(255) NOT NULL,
  google_id VARCHAR(255),
  email_verified BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT uq_users_project_email UNIQUE (project_id , email),
  CONSTRAINT uq_users_project_google UNIQUE (project_id, google_id)
);

CREATE INDEX IF NOT EXISTS idx_users_project_email ON users(project_id, email);

