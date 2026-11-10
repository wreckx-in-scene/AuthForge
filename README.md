# AuthForge

![CI](https://github.com/wreckx-in-scene/AuthForge/actions/workflows/ci.yml/badge.svg)

AuthForge is a self-hosted, multi-tenant authentication platform developer
preview. It lets developers build an app's user authentication through an
environment-scoped API—or delegate sign-in to AuthForge using hosted OpenID
Connect (OIDC).

## What it does

- Manages developer accounts, organizations, projects, environments, API keys,
  and OAuth clients through a web console.
- Provides application-user password signup/login, refresh-token rotation,
  logout, email verification, and password recovery; Google sign-in is optional
  and requires platform credentials.
- Supports hosted OIDC password sign-in/signup with authorization code + PKCE,
  one-use authorization codes, EdDSA ID tokens, discovery, and JWKS.
- Includes Go and TypeScript SDKs, a browser API demo, and a runnable Node.js
  OIDC relying-party example.

## Architecture

Five Go services run in Docker Compose. The gateway exposes the HTTP API,
console, and OIDC endpoints; internal service communication uses gRPC.

```text
Applications ── HTTPS/JSON ──> Gateway ── gRPC ──> Tenant
                                  │
                                  └── gRPC ──> Auth ── gRPC ──> Identity
                                                 └──── gRPC ──> Session

Tenant, Identity, Session ── owned schemas ──> PostgreSQL
```

Tenant, application-user, and session data are separated by service-owned
schemas. Application identity and session tables use PostgreSQL row-level
security. Passwords are hashed with Argon2id; access and ID tokens are signed
with EdDSA.

## Run locally

Requirements: Docker Compose v2.

```bash
docker compose up --build
```

- Console: [http://localhost:8080](http://localhost:8080)
- Application API demo: [http://localhost:8080/demo](http://localhost:8080/demo)
- Mailpit (local verification/reset emails): [http://localhost:8025](http://localhost:8025)

The local Compose configuration uses development credentials and open console
signup. Do not expose it to the internet.

## Project scope

This repository demonstrates end-to-end developer onboarding and application
authentication, including a sample OIDC integration. It is a runnable
developer preview—not a hosted or production-ready identity provider. The
included beta deployment is a single-VM Compose setup, not highly available;
it requires operator-provided infrastructure, HTTPS, secrets, and SMTP. The
beta overlay supports a shared console signup code, not individual invitations.

Production use would additionally require operational safeguards such as
managed/rotatable signing keys, authenticated service-to-service traffic,
shared rate limiting, automated backups and restore drills, monitoring, and an
independent security review. See [architecture and production gaps](docs/architecture.md)
and the [beta deployment guide](docs/beta-deployment.md).

## Development checks

```bash
go test ./...
go vet ./...
buf lint

cd sdk/typescript
npm test

cd ../../examples/node-oidc
npm test
```

## Documentation

- [Quickstart](docs/quickstart.md) · [HTTP API](docs/api.md) ·
  [Architecture and design decisions](docs/architecture.md) ·
  [Deployment guide](docs/beta-deployment.md)
- [Go SDK](sdk/go/README.md) · [TypeScript SDK](sdk/typescript/README.md) ·
  [Node.js OIDC example](examples/node-oidc/README.md)
