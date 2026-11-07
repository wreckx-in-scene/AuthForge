# HTTP API

The gateway listens on port 8080. Console routes take `Authorization: Bearer <console token>`. Application routes take `X-Api-Key`. `GET /v1/me` takes an application access token.

Tokens are returned once. API keys and refresh tokens are not recoverable later.

## Password reset

`POST /v1/password-reset/request`

```json
{ "email": "ada@campus.edu" }
```

Returns `{"status":"ok"}` whether or not the account exists. If it exists, AuthForge emails a single-use link that expires after 30 minutes. The request response never contains the reset token.

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

## Application auth

`POST /v1/signup` and `POST /v1/login`

```json
{ "email": "student@campus.edu", "password": "correct-horse" }
```

The environment comes from the API key, not the body.

`POST /v1/token/refresh`

```json
{ "refresh_token": "af_rt_..." }
```

Reusing an already rotated refresh token returns `401` and `refresh_reuse_detected`. The session and the rest of that token family are revoked.

`POST /v1/logout`

Send the refresh token, or send an access token in `Authorization` and an empty JSON object. Logout revokes the session. An access token that was already issued stays valid until `expires_at`.

`GET /v1/me` checks the access-token signature, issuer, and expiry. It does not read the session row.

## Errors

Failures use `{ "error": "<code>" }`.

| Code | Meaning |
|---|---|
| `invalid_argument` | Missing or malformed field, including a short password |
| `invalid_credentials` | Unknown email or wrong password |
| `invalid_api_key` | Missing or unknown `X-Api-Key` |
| `email_already_registered` | Signup lost the unique-email race, or the email is taken |
| `slug_taken` | Organization or project slug already exists |
| `forbidden` | The console account is not a member |
| `not_found` | Project or environment does not exist |
| `account_disabled` | The application user is not active |
| `refresh_reuse_detected` | A used refresh token was presented |
| `unauthenticated` | Console or access token was rejected |
