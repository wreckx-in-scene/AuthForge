# AuthForge

Authentication infrastructure for multi-tenant applications. Developers create an organization, a project, and an environment, then call the HTTP API with an environment key. Internal services talk gRPC.

The design notes are in [docs/architecture.md](docs/architecture.md) and [docs/decisions.md](docs/decisions.md). The HTTP contract is in [docs/api.md](docs/api.md).

## Run

```bash
docker compose up --build
```

The gateway is at `http://localhost:8080`. Postgres is published on `localhost:5432` for local inspection. The gRPC services are only on the Compose network.
Local password-reset and email-verification messages are captured by Mailpit at `http://localhost:8025`.
Open `http://localhost:8080` for the developer console to create an account, organization, project, environment, and publishable API key.

The signing seed in `docker-compose.yml` is a development key. It is not a production secret.
Google sign-in is disabled unless `GOOGLE_CLIENT_ID` is configured for the platform-owned Google Identity Services app.

## TypeScript SDK

The application-user API has a framework-agnostic TypeScript client in [`sdk/typescript`](./sdk/typescript). See its [usage guide](./sdk/typescript/README.md) for install, signup/login, Google sign-in, email verification, and token management. Use a publishable environment key in browser code; never bundle a secret key.

## First path

```bash
curl -s -X POST localhost:8080/v1/console/signup -H "content-type: application/json" -d "{\"email\":\"ada@campus.edu\",\"password\":\"correct-horse\",\"name\":\"Ada\"}"
```

Use the returned console token as `Authorization: Bearer ...` for `/v1/organizations`, `/v1/projects`, `/v1/environments`, and `/v1/api-keys`. Signup and login for an application user send that API key in `X-Api-Key`.

## Password reset

Console accounts can request a password reset email through the gateway. The request response is generic and never includes the token. In the local Compose stack, open Mailpit at `http://localhost:8025` to retrieve the message. The link base is configured with `RESET_PASSWORD_URL`; in production, point it at the console UI over HTTPS. This repo does not include that UI yet—the page should read the link's `token` query parameter and submit it to the confirm endpoint.

```bash
curl -s -X POST localhost:8080/v1/password-reset/request -H "content-type: application/json" -d "{\"email\":\"ada@campus.edu\"}"
curl -s -X POST localhost:8080/v1/password-reset/confirm -H "content-type: application/json" -d "{\"reset_token\":\"af_pr_...\",\"password\":\"new-password-123\"}"
```

## Email verification

Application signup sends a verification email. For local development, open Mailpit at `http://localhost:8025`; the configured link points at `http://localhost:3000/verify-email`, which a frontend must implement. The page should submit the link's `environment_id` and `token` to the gateway:

```bash
curl -s -X POST localhost:8080/v1/email/verification/confirm -H "content-type: application/json" -d "{\"environment_id\":\"<uuid>\",\"token\":\"af_ev_...\"}"
```

To resend a verification link, send `POST /v1/email/verification/request` with the environment API key and `{"email":"student@campus.edu"}`.

## Google sign-in

Configure the platform-owned Google Identity Services OAuth client ID as `GOOGLE_CLIENT_ID` in the auth service environment. In the client, request a one-time nonce with the environment API key, pass that nonce to Google Identity Services, then exchange the returned ID token and nonce:

```bash
curl -s -X POST localhost:8080/v1/oauth/google/nonce -H "X-Api-Key: <environment-key>"
curl -s -X POST localhost:8080/v1/oauth/google -H "X-Api-Key: <environment-key>" -H "content-type: application/json" -d "{\"id_token\":\"<google-id-token>\",\"nonce\":\"<nonce-from-first-call>\"}"
```

Google credentials are validated against the configured client ID and Google's OIDC issuer. Only Google-verified email addresses are accepted. Nonces expire after five minutes and can be exchanged once. Google accounts are linked to an existing application user with the same verified email, or create a new verified user. The browser/client UI is not included yet.

## Tests

```bash
go test ./...
```

Password hashing tests are slow on purpose. Argon2id is configured at 64 MiB.
