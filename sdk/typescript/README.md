# AuthForge TypeScript SDK

Framework-agnostic client for AuthForge's application-user HTTP API. It supports
password signup/login, Google ID-token exchange, email verification, refresh,
logout, and the current-user endpoint. Console account and project-management
routes are intentionally out of scope.

## Install

```bash
npm install ./sdk/typescript
```

The package is not published to npm yet.

## Usage

Use an environment's **publishable** API key in browser code. Never put a
secret API key in a browser bundle. The SDK does not persist tokens; choose
storage appropriate to your application and pass the current access or refresh
token to the relevant method.

```ts
import { AuthForgeClient } from "authforge-client";

const auth = new AuthForgeClient({
  apiKey: "af_test_...",
  baseUrl: "http://localhost:8080",
});

const session = await auth.signUp({
  email: "ada@example.com",
  password: "a-long-password",
});
const user = await auth.getCurrentUser(session.access_token);
```

### Password login and token refresh

```ts
const session = await auth.logIn({
  email: "ada@example.com",
  password: "a-long-password",
});
const refreshed = await auth.refresh(session.refresh_token);
```

Refresh tokens rotate on every successful refresh. Store the returned refresh
token instead of reusing the old one.

### Google Identity Services

Request a nonce from AuthForge, provide it to Google Identity Services when
requesting the Google ID token, then exchange that token:

```ts
const nonce = await auth.requestGoogleNonce();
// Obtain an ID token from Google Identity Services using this nonce.
const session = await auth.signInWithGoogle(googleIdToken, nonce);
```

The platform must have `GOOGLE_CLIENT_ID` configured for this flow.

### Email verification and logout

```ts
await auth.requestEmailVerification("ada@example.com");
await auth.verifyEmail(environmentId, verificationToken);
await auth.logOut({ refreshToken: session.refresh_token });
```

`verifyEmail` takes an environment ID and a token from the verification link.
Alternatively, log out with an access token:

```ts
await auth.logOut({ accessToken: session.access_token });
```

### Errors

API failures throw `AuthForgeError`, which exposes the HTTP `status`, server
`code`, and (for rate limits) the numeric `retryAfter` duration in seconds.
Network errors are surfaced unchanged.

## Development

```bash
npm install
npm test
```
