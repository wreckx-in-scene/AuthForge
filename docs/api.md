# HTTP API

The gateway listens on port 8080. Console routes take `Authorization: Bearer <console token>`. Application routes take `X-Api-Key`. `GET /v1/me` takes an application access token.

Tokens are returned once. API keys and refresh tokens are not recoverable later.

## Rate limits

The gateway applies per-source-IP token-bucket limits to authentication endpoints: signup, login, and hosted authorization have a burst of 10 and refill at 10 per minute; token exchanges have a burst of 20 per minute; password-reset and verification email requests share a burst of 3 and refill at 3 per 15 minutes; reset and verification confirmations share a burst of 5 and refill at 5 per minute. Requests exceeding a limit return `429` with `{ "error": "rate_limited" }` and a `Retry-After` header. The gateway uses the TCP peer address and does not trust `X-Forwarded-For`.

These limits are in-memory and apply per gateway process. Multi-instance deployments need a shared limiter, and deployments behind a reverse proxy need trusted-proxy client IP handling before the gateway can accurately apply per-client limits.

## Password reset

`POST /v1/password-reset/request`

```json
{ "email": "ada@campus.edu" }
```

Returns `{"status":"ok"}` whether or not the account exists. If it exists, AuthForge emails a single-use link that expires after 30 minutes. The request response never contains the reset token.
The developer console includes the request form and serves the reset page at `/reset-password`; configure `RESET_PASSWORD_URL` to that HTTPS URL in production.

`POST /v1/password-reset/confirm`

```json
{ "reset_token": "af_pr_...", "password": "new-password-123" }
```

An expired, unknown, or previously used token returns `401` with `invalid_reset_token`. Password reset does not automatically log the account in.

## Console

`POST /v1/console/signup`

```json
{ "email": "ada@campus.edu", "password": "correct-horse", "name": "Ada" }
```

`POST /v1/console/login`

```json
{ "email": "ada@campus.edu", "password": "correct-horse" }
```

Both return `access_token`, `expires_at`, and `account`.

The gateway also serves a first-party developer console at `/`. It uses these console endpoints to create an account and initial workspace, then browse and manage the account's organizations, projects, environments, API keys, and registered OAuth clients. The console token is kept in tab-scoped session storage and is cleared by Sign out; signing out in the current API does not revoke the stateless token, which remains valid until its expiry. The console issues publishable keys only and displays each returned key once.

`POST /v1/organizations`

```json
{ "name": "CampusConnect", "slug": "campus-connect" }
```

`POST /v1/projects`

```json
{ "organization_id": "<uuid>", "name": "Campus Connect", "slug": "campus-connect" }
```

`POST /v1/environments`

```json
{ "project_id": "<uuid>", "name": "development" }
```

`name` is a slug. `production` issues `af_live_` keys. Every other name issues `af_test_` keys.

`POST /v1/api-keys`

```json
{ "environment_id": "<uuid>", "key_class": "publishable" }
```

`key_class` is `publishable` or `secret`. The `key` field is the only copy of the secret.

All workspace management routes below require the console bearer token and return only resources the authenticated account is authorized to access:

`GET /v1/organizations` returns `{ "organizations": [{ "id", "name", "slug" }] }`.

`GET /v1/organizations/{organizationID}/projects` returns `{ "projects": [{ "id", "organization_id", "name", "slug" }] }`.

`GET /v1/projects/{projectID}/environments` returns `{ "environments": [{ "id", "project_id", "organization_id", "name" }] }`.

`GET /v1/environments/{environmentID}/api-keys` returns key metadata in `{ "keys": [...] }`: each item includes `id`, `environment_id`, `key_class`, `prefix`, `status`, and `created_at` (Unix seconds). It never returns key secrets.

`DELETE /v1/api-keys/{keyID}` revokes an account-owned key and returns `{ "revoked": true }`. Revocation is permanent; an already revoked key remains revoked.

OAuth client registration is scoped to an environment and is managed with the console bearer token:

`POST /v1/oauth-clients`

```json
{
  "environment_id": "<uuid>",
  "name": "Campus Connect web",
  "redirect_uris": ["https://app.example.com/auth/callback", "http://localhost:3000/auth/callback"]
}
```

Redirect URIs must be absolute HTTPS URLs, or HTTP URLs on localhost/loopback for development. Fragments and user-info are rejected. The callback list is capped at 10 entries and will be matched exactly by the hosted authorization flow. The response includes a public `client_id`; it is not a secret.

`GET /v1/environments/{environmentID}/oauth-clients` returns active registered clients. `DELETE /v1/oauth-clients/{clientID}` disables a client and returns `{ "deleted": true }`.

Client registration is available in the developer console. The public client ID is safe to use in browser code; do not treat it as a secret.

## Hosted OpenID Connect

The gateway publishes `GET /.well-known/openid-configuration` and `GET /.well-known/jwks.json`. Configure `ISSUER` as the public base URL over HTTPS in production; the discovery issuer must exactly match the `iss` claim in AuthForge ID tokens.

Register exact callback URLs on the OAuth client, then redirect the browser to `/authorize` with:

- `client_id`, the registered public client ID
- `redirect_uri`, an exact registered callback URL
- `response_type=code`
- `scope=openid%20email`
- a unique `state`
- `code_challenge` and `code_challenge_method=S256`
- optional `nonce`

AuthForge validates the client and callback before displaying its hosted password sign-in/sign-up page. The callback receives only a short-lived, single-use authorization code and the original state; access and refresh tokens are never put in the redirect URL.

Exchange the code at `POST /v1/token` using `application/x-www-form-urlencoded`:

```text
grant_type=authorization_code
client_id=<public-client-id>
code=<authorization-code>
redirect_uri=<same-registered-callback>
code_verifier=<original-pkce-verifier>
```

The response contains an AuthForge access token, refresh token, and OIDC ID token. The access token is for the AuthForge environment API; the ID token is audience-bound to the OAuth client and includes `sub`, `email`, `email_verified`, and the request `nonce` when supplied. Codes expire after five minutes and are bound to the client, callback, and S256 verifier. The token endpoint accepts browser requests only when their `Origin` matches a registered callback origin; server-side exchanges may omit `Origin`.

The initial hosted screen supports password sign-in and signup. Supported scopes are `openid email`; social sign-in, userinfo, logout redirect, and broader scope/claim customization are not included yet.

## Application auth

`POST /v1/signup` and `POST /v1/login`

```json
{ "email": "student@campus.edu", "password": "correct-horse" }
```

The environment comes from the API key, not the body.

`POST /v1/oauth/google/nonce`

Requires the environment's `X-Api-Key`. Returns a one-time nonce valid for five minutes. Pass this nonce to Google Identity Services and include it again when exchanging the returned ID token.

`POST /v1/oauth/google`

Requires the environment's `X-Api-Key`.

```json
{ "id_token": "<google-id-token>", "nonce": "<one-time-nonce>" }
```

Returns the standard AuthForge access and refresh token response. The ID token must have the configured platform Google client ID as audience, a valid Google OIDC signature and issuer, a verified email, and the matching unused nonce. Google sign-in returns `503` with `google_sign_in_unavailable` until `GOOGLE_CLIENT_ID` is configured.

New application users receive an email verification link automatically. Verification links expire after 24 hours and work once. `email_verified` remains false in tokens issued before verification; sign in or refresh after verifying to receive a token with the updated claim.

`POST /v1/email/verification/request`

Requires the environment's `X-Api-Key`.

```json
{ "email": "student@campus.edu" }
```

Returns `{"status":"ok"}` whether or not a matching user exists.

`POST /v1/email/verification/confirm`

```json
{ "environment_id": "<uuid>", "token": "af_ev_..." }
```

Returns `{"verified":true}` on success. Expired, unknown, or previously used tokens return `401` with `invalid_verification_token`.
The self-hosted developer console also serves `/verify-email`, which reads the `environment_id` and `token` query parameters from the email link and confirms them with this endpoint.

`POST /v1/token/refresh`

```json
{ "refresh_token": "af_rt_..." }
```

Reusing an already rotated refresh token returns `401` and `refresh_reuse_detected`. The session and the rest of that token family are revoked.

`POST /v1/logout`

Send the refresh token, or send an access token in `Authorization` and an empty JSON object. Logout revokes the session. An access token that was already issued stays valid until `expires_at`.

`GET /v1/me` checks the access-token signature, issuer, and expiry. It does not read the session row.

## TypeScript SDK

The framework-agnostic client in [`sdk/typescript`](../sdk/typescript) wraps the application-user API: password signup/login, Google ID-token exchange, email verification, refresh, logout, and `GET /v1/me`. It does not include console account or project-management routes and does not persist tokens. Use only a publishable environment key in browser code. See the [SDK guide](../sdk/typescript/README.md) for examples.

## Errors

Failures use `{ "error": "<code>" }`.

| Code | Meaning |
|---|---|
| `invalid_argument` | Missing or malformed field, including a short password |
| `invalid_redirect_uri` | OAuth client redirect URI is not an allowed absolute HTTPS or loopback URL |
| `invalid_credentials` | Unknown email or wrong password |
| `invalid_api_key` | Missing or unknown `X-Api-Key` |
| `email_already_registered` | Signup lost the unique-email race, or the email is taken |
| `slug_taken` | Organization or project slug already exists |
| `forbidden` | The console account is not a member |
| `not_found` | Project or environment does not exist |
| `account_disabled` | The application user is not active |
| `refresh_reuse_detected` | A used refresh token was presented |
| `invalid_verification_token` | Email verification token is invalid, expired, or already used |
| `invalid_google_credential` | Google ID token, verified email, or one-time nonce was rejected |
| `google_identity_conflict` | Google identity is already linked to a different account |
| `google_sign_in_unavailable` | Google sign-in is disabled because no platform client ID is configured |
| `rate_limited` | Too many requests; wait for the `Retry-After` duration |
| `unauthenticated` | Console or access token was rejected |
