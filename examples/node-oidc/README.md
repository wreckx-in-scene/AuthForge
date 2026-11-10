# AuthForge hosted sign-in with Node.js

This runnable relying-party example lets a Node.js web app delegate sign-in to
AuthForge. It uses only Node.js built-ins and demonstrates authorization code
with PKCE, state and nonce checks, ID-token signature validation using
AuthForge's JWKS, an HttpOnly application session cookie, token refresh, and
sign-out.

The sample is intentionally local-development-only: its sessions and pending
login transactions are in memory, and it has no persistence, CSRF framework,
or production deployment configuration.

## 1. Start AuthForge

From the repository root:

```bash
docker compose up --build
```

Open `http://localhost:8080` and create a developer account, organization,
project, and **development** environment. Keep the publishable API key private
to this walkthrough; the hosted OIDC app uses the OAuth client ID, not an API
key.

## 2. Register the application

In the selected environment's **OAuth Clients** section, register:

- Application name: `My Node app`
- Callback URL: `http://localhost:3000/auth/callback`

Copy the client ID. The callback URL must match exactly.

## 3. Configure and run this app

In a second terminal, from the repository root:

PowerShell:

```powershell
$env:AUTHFORGE_ISSUER = "http://localhost:8080"
$env:AUTHFORGE_CLIENT_ID = "<client-id-from-the-console>"
$env:AUTHFORGE_REDIRECT_URI = "http://localhost:3000/auth/callback"
Set-Location examples\node-oidc
npm test
npm start
```

macOS/Linux:

```bash
export AUTHFORGE_ISSUER=http://localhost:8080
export AUTHFORGE_CLIENT_ID="<client-id-from-the-console>"
export AUTHFORGE_REDIRECT_URI=http://localhost:3000/auth/callback
cd examples/node-oidc
npm test
npm start
```

Open `http://localhost:3000`, select **Sign in or create an account**, and use
the **Create account** tab on AuthForge's hosted page for a new application
user. AuthForge sends a verification message to local Mailpit at
`http://localhost:8025`. The example displays the verified status returned by
the validated ID token; after verifying, sign in again to see
`email_verified: true`.

## What this integration does

- Creates fresh state, nonce, and an S256 PKCE verifier for every login.
- Keeps the pending transaction on the app server and correlates it with a
  SameSite, HttpOnly cookie.
- Exchanges the authorization code from the app server, not from browser
  JavaScript.
- Checks the ID-token EdDSA signature, issuer, audience, timestamps, nonce,
  subject, email, and `email_verified` claim using the discovery document and
  JWKS.
- Keeps AuthForge access and refresh tokens in the app server's in-memory
  session store; the browser receives only an opaque HttpOnly session cookie.
- Refreshes expiring access tokens before the app calls AuthForge's `/v1/me`
  endpoint and revokes the refresh token on sign-out.

No client secret or AuthForge API key is needed for this public OIDC client.
Never put an OAuth client secret, refresh token, or service credential in
browser code.

For production, use a maintained session store, HTTPS-only cookies, your
framework's CSRF/session protections, secure secret and key management, and
an established OIDC library appropriate to your framework. Configure
`AUTHFORGE_ISSUER` as the exact public HTTPS issuer used in ID tokens.
