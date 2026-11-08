# AuthForge

Authentication infrastructure for multi-tenant applications. Developers create an organization, a project, and an environment, then call the HTTP API with an environment key. Internal services talk gRPC.

The design notes are in [docs/architecture.md](docs/architecture.md) and [docs/decisions.md](docs/decisions.md). The HTTP contract is in [docs/api.md](docs/api.md).

## Run

```bash
docker compose up --build
```

The gateway is at `http://localhost:8080`. Postgres is published on `localhost:5432` for local inspection. The gRPC services are only on the Compose network.
Local password-reset and email-verification messages are captured by Mailpit at `http://localhost:8025`.
Open `http://localhost:8080` for the developer console to create an account, organization, project, environment, and publishable API key. Open `http://localhost:8080/demo` to try application signup, login, verification, refresh, and logout with a development key.

## Developer preview

This is a self-hosted developer preview, not a production-ready hosted service. The Compose defaults use development credentials and local Mailpit; Google sign-in needs a platform `GOOGLE_CLIENT_ID`. The console currently supports a single developer account and does not provide team invitations or roles. Before exposing a deployment to real users, configure HTTPS, production secrets and SMTP, backups, and appropriate network/security controls.

The gateway now supports hosted OpenID Connect password sign-in/signup using authorization code + PKCE. Register an OAuth client and exact HTTPS callback URLs in the console, then use the discovery document at `/.well-known/openid-configuration`; see the [API guide](./docs/api.md#hosted-openid-connect) for the authorize and token requests. The first hosted screen supports email/password; social sign-in and userinfo are not included yet.

The in-browser developer demo is deliberately limited to `af_test_pk_` keys and keeps the application key and tokens in page memory only. Use it to explore the application API, not as your application. For an integration, use the [TypeScript SDK](./sdk/typescript/README.md) or call the API from your own server/client.

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

Console accounts can request a password reset from the sign-in screen. The request response is generic and never reveals whether an account exists. In the local Compose stack, open Mailpit at `http://localhost:8025` to retrieve the message; its reset link opens the console at `http://localhost:8080/reset-password`. The page submits the link's `token` query parameter to the confirm endpoint. Set `RESET_PASSWORD_URL` to the console reset page over HTTPS in production.

```bash
curl -s -X POST localhost:8080/v1/password-reset/request -H "content-type: application/json" -d "{\"email\":\"ada@campus.edu\"}"
curl -s -X POST localhost:8080/v1/password-reset/confirm -H "content-type: application/json" -d "{\"reset_token\":\"af_pr_...\",\"password\":\"new-password-123\"}"
```

## Email verification

Application signup sends a verification email. For local development, open Mailpit at `http://localhost:8025`; the link opens the hosted verification page at `http://localhost:8080/verify-email`, which confirms the link's `environment_id` and `token` with the gateway. The developer demo can also request a fresh verification link. Applications can alternatively implement their own verification page:

```bash
curl -s -X POST localhost:8080/v1/email/verification/confirm -H "content-type: application/json" -d "{\"environment_id\":\"<uuid>\",\"token\":\"af_ev_...\"}"
```

To resend a verification link, the developer demo has a resend button, or send `POST /v1/email/verification/request` with the environment API key and `{"email":"student@campus.edu"}`.

## Google sign-in

Configure the platform-owned Google Identity Services OAuth client ID as `GOOGLE_CLIENT_ID` in the auth service environment. In the client, request a one-time nonce with the environment API key, pass that nonce to Google Identity Services, then exchange the returned ID token and nonce:

```bash
curl -s -X POST localhost:8080/v1/oauth/google/nonce -H "X-Api-Key: <environment-key>"
curl -s -X POST localhost:8080/v1/oauth/google -H "X-Api-Key: <environment-key>" -H "content-type: application/json" -d "{\"id_token\":\"<google-id-token>\",\"nonce\":\"<nonce-from-first-call>\"}"
```

Google credentials are validated against the configured client ID and Google's OIDC issuer. Only Google-verified email addresses are accepted. Nonces expire after five minutes and can be exchanged once. Google accounts are linked to an existing application user with the same verified email, or create a new verified user. Google sign-in is available through the TypeScript SDK; the self-hosted demo currently covers password-based flows.

## Tests

```bash
go test ./...
```

Password hashing tests are slow on purpose. Argon2id is configured at 64 MiB.
