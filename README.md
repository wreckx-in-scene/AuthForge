# AuthForge

Authentication infrastructure for multi-tenant applications. Developers create an organization, a project, and an environment, then call the HTTP API with an environment key. Internal services talk gRPC.

The design notes are in [docs/architecture.md](docs/architecture.md) and [docs/decisions.md](docs/decisions.md). The HTTP contract is in [docs/api.md](docs/api.md).

## Run

```bash
docker compose up --build
```

The gateway is at `http://localhost:8080`. Postgres is published on `localhost:5432` for local inspection. The gRPC services are only on the Compose network.

The signing seed in `docker-compose.yml` is a development key. It is not a production secret.

## First path

```bash
curl -s -X POST localhost:8080/v1/console/signup -H "content-type: application/json" -d "{\"email\":\"ada@campus.edu\",\"password\":\"correct-horse\",\"name\":\"Ada\"}"
```

Use the returned console token as `Authorization: Bearer ...` for `/v1/organizations`, `/v1/projects`, `/v1/environments`, and `/v1/api-keys`. Signup and login for an application user send that API key in `X-Api-Key`.

## Password reset

Console accounts can request a password reset token through the gateway. The token is short-lived and invalid after use or expiry.

```bash
curl -s -X POST localhost:8080/v1/password-reset/request -H 'content-type: application/json' -d '{"email":"ada@campus.edu"}'
curl -s -X POST localhost:8080/v1/password-reset/confirm -H 'content-type: application/json' -d '{"reset_token":"af_pr_...","password":"new-password-123"}'
```

## Tests

```bash
go test ./...
```

Password hashing tests are slow on purpose. Argon2id is configured at 64 MiB.
