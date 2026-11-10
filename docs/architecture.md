# AuthForge architecture

AuthForge is a multi-tenant identity platform. A developer creates an organization, a project, and separate development and production environments, then points an application at an environment API key. The application uses AuthForge for signup, login, refresh, and session revocation.

Application users are not organization members. Organization members are the developers who manage a project. Those two identity domains never share a table.

## Delivered capabilities

The developer preview has two end-to-end paths:

1. **Environment API:** a developer creates a console account, organization,
   project, environment, and API key. An application uses that key for
   password signup/login, email verification, Google sign-in (when configured),
   token refresh, logout, and the current-user endpoint.
2. **Hosted OIDC:** a developer registers exact callback URLs for a public
   OAuth client. The application redirects users to AuthForge for password
   signup/login using authorization code + PKCE, validates the returned ID
   token against discovery/JWKS, and keeps its own application session.

The gateway publishes OIDC discovery, JWKS, authorization, and token
endpoints. Authorization codes are short lived, single use, and bound to the
client, callback URL, and S256 challenge. Email verification and console
password recovery use separate single-use SMTP links. A runnable relying-party
integration is in `examples/node-oidc`.

The project is a self-hosted developer preview, not a production hosted
identity service. Compose credentials, local Mailpit, in-memory rate limits,
and the sample app's in-memory sessions are for development only.

## Production readiness boundary

Before operating this as an internet-facing identity service, the deployment
needs, at minimum:

- Managed secrets and signing-key storage, key rotation, and recovery procedures.
- HTTPS termination and authenticated/encrypted service-to-service traffic.
- Production SMTP, delivery monitoring, and abuse-resistant account recovery.
- Shared rate limiting with explicitly trusted proxy/client-IP configuration.
- Automated database backups, restore drills, monitoring, alerting, and an
  incident-response process.
- A security review of authentication flows and deployment configuration.
- Durable developer sessions, team membership/roles, and operational support
  if it is offered to multiple customer teams.

The Node.js relying-party example is also intentionally a teaching sample:
its sessions and login transactions live in memory. Use a maintained
framework session store and OIDC library for a real application.

## Runtime

```text
developer app
    |  HTTPS JSON
API gateway
    |  gRPC
    +-- tenant       organizations, projects, environments, API keys, console accounts, password recovery
    +-- auth         signup, login, refresh, logout, authorization, token signing
            +-- identity    users and password credentials
            +-- session     sessions and refresh-token families
```

Hosted OIDC is served by the gateway. It resolves a registered public client
and exact callback before displaying the hosted password page; auth
orchestrates identity/session work and signs an ID token whose audience is the
OAuth client. The current implementation uses the same Ed25519 signer for
application access tokens and ID tokens; JWKS publishes its public key. A
production key-rotation design must cover both token types and all verifiers.

Postgres is the primary data dependency. The tenant service sends console password-reset messages and the identity service sends application-user verification messages through configured SMTP; local Compose captures them in Mailpit. Each service owns a schema: `tenant`, `identity`, and `session`. The auth service and the gateway do not own tables.

The local Compose stack connects application services as the non-superuser
`authforge_app` role; the migration job alone uses the PostgreSQL
administrator. `docs/beta-deployment.md` describes an opt-in, single-VM Compose
overlay with Caddy HTTPS, externally configured SMTP, and a separately
provisioned application-role password. It remains a limited, non-HA beta
deployment and depends on infrastructure the operator supplies.

The gateway rate-limits signup, login, reset, and verification endpoints by the direct TCP peer IP. This first implementation is process-local; horizontal deployments need a shared limiter and an explicitly trusted proxy boundary before forwarding client IPs.

Google sign-in uses a platform-owned client ID. The gateway binds each ID-token exchange to the environment selected by the API key; the auth service verifies Google's OIDC signature, issuer, audience, verified-email claim, and a one-time five-minute nonce. Identity stores the provider subject separately from credentials and links an existing user only on a verified matching email.

| Service   | Public port | Talks to                          |
|-----------|-------------|-----------------------------------|
| gateway   | 8080        | tenant, auth                      |
| auth      | internal    | identity, session                 |
| tenant    | internal    | its own schema                    |
| identity  | internal    | its own schema                    |
| session   | internal    | its own schema                    |

Internal ports are not published on the host. A client that can reach Postgres still cannot call `CreateUser` directly unless it is also inside the Compose network.

Password reset tokens are random, expire after 30 minutes, and are stored only as SHA-256 hashes. A new request invalidates the previous token for that console account. The gateway always returns the same success response for known and unknown addresses, and the token is delivered only in the email link.

Application-user verification tokens are random, expire after 24 hours, and are stored only as SHA-256 hashes in the identity schema under environment RLS. Verification sets `email_verified` once; clients must sign in or refresh to receive a newly signed token containing the updated claim. Verification requests also return the same success response for known and unknown addresses.

## Two credentials, two callers

Console routes authenticate a developer account. The gateway checks a short-lived HMAC token and sends the account id to the tenant service. The tenant service checks organization membership before it creates a project, an environment, or an API key.

Application routes authenticate an environment. The caller sends `X-Api-Key`. The gateway resolves that key to one organization, project, and environment, then calls auth with those ids. The signup body contains an email and a password. It does not contain an environment id, so a caller cannot aim a key at a different environment.

`GET /v1/me` is different again. It requires an access token for an application user.

## Signup and login

Auth is the orchestrator. It does not keep a copy of the user or the session.

1. The gateway resolves the API key.
2. Auth asks identity to create the user, or to check the password.
3. After identity commits, auth asks session to create a session and a refresh token.
4. Auth signs an access token and returns both tokens.
5. If signing fails, auth revokes the session it just created and returns an error. The client never receives a refresh token for a session the signer could not complete.

Identity commits before the session exists. If the session call fails, the user row remains and no tokens are returned. The client can log in. A second signup for the same email returns a conflict and does not change the password.

There is no distributed transaction. The durable point of no return is the user row. Tokens are returned only after the session row commits.

Login uses the same session and signing steps. An unknown email and a wrong password return the same error. Identity still runs a password hash when the email is missing, so the two cases take a similar amount of time.

## Access tokens and refresh tokens

Access tokens are Ed25519 JWTs. The private seed stays on the auth service. On startup the gateway fetches the public key and keeps it in memory. Verification of `GET /v1/me` does not call the database.

The claims are:

| Claim            | Meaning                                              |
|------------------|------------------------------------------------------|
| `iss`            | configured issuer                                    |
| `sub`            | application user id                                  |
| `aud`            | environment id                                       |
| `sid`            | session id                                           |
| `pid`            | project id                                           |
| `email_verified` | false until the application user verifies their email |
| `exp`            | ten minutes by default                               |

Refresh tokens are opaque random strings with an `af_rt_` prefix. The session service stores only a SHA-256 hash. Rotation locks the presented token and its session, marks the token used, and inserts a successor whose `parent_id` points at the used token. The session expiry slides forward with the new token.

If a presented token was already used, the session service revokes the session and every refresh token in that family, then returns `refresh_reuse_detected`. A later attempt with the successor fails as well. Logout is not reuse: a revoked token that was never used is rejected without treating the caller as a thief.

A revoked session does not invalidate an access token that has already been signed. `GET /v1/me` keeps working until `exp`. Refresh and logout consult the session row. That split is deliberate: revocation is enforced on the refresh token, and access tokens are short lived so the window stays small.

## Environment isolation

An API key belongs to one environment. Users, credentials, sessions, and refresh tokens store that environment id. Uniqueness for a login is `(environment_id, normalized_email)`.

Development and production are different environments under the same project, so a signup in development does not create a production user. A key for environment A cannot read or write environment B. Cross-project access fails for the same reason: an environment belongs to one project.

Identity and session enable Postgres row-level security and force it. The application role is not a superuser and does not bypass RLS. Each transaction sets `authforge.environment_id` before it touches those tables. If the setting is missing, the policy matches no rows.

Refresh rotation is the exception to "the caller already knows the environment." The refresh token is the credential. A `SECURITY DEFINER` function resolves the hash to an environment id, the transaction sets the RLS setting, and only then locks the row. The function returns an id for a hash the caller already possesses.

The tenant schema does not use RLS. Console queries are membership queries across an organization, and the tenant service expresses that rule in SQL.

## Consistency choices that are easy to get wrong

Concurrent signup is settled by the unique index, not by a lock in the auth service. One transaction inserts the user. The other gets a unique violation and returns a conflict.

Refresh reuse is settled inside one database transaction. The decision uses the locked row, not the snapshot from the earlier hash lookup.

A lost refresh response can cause a false reuse detection. The client retries the old token, the family is revoked, and the user logs in again. The alternative is a grace window that re-issues the same successor. That requires storing a recoverable copy of the new token. This milestone fails closed instead.

Orphan sessions are possible if the process dies after the session commit and before the response is written. They expire with the refresh lifetime. They do not authorize a caller who never received the token.

## Local security boundaries

Compose is a development topology.

- The private Ed25519 seed is injected into the auth container. It is not stored in Postgres.
- The console HMAC secret is shared by the tenant service and the gateway. It cannot sign application access tokens, and the access-token verifier accepts only EdDSA.
- API keys and refresh tokens are hashed with SHA-256. They are 256-bit random values, so a password-stretching function would add cost without a lookup key.
- Passwords are hashed with Argon2id. The parameters travel with the hash in a PHC string.
- Service-to-service calls use plaintext gRPC on the Compose network. mTLS is the production requirement, not part of this local stack.
- The gateway does not log request bodies, authorization headers, API keys, or tokens.
