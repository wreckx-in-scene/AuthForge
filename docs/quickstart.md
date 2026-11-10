# AuthForge Developer Quickstart

Get authentication running in your application in under 5 minutes.

---

## 1. Get Your Workspace & API Key

1. Start AuthForge locally (or navigate to your hosted deployment):
   ```bash
   docker compose up -d
   ```
2. Open the **Developer Console** at [http://localhost:8080](http://localhost:8080).
3. Create an organization (e.g. `Acme Corp`), project (e.g. `campus-connect`), and select the `development` environment.
4. Issue a **Publishable API Key** (starts with `af_test_pk_...`).

---

## 2. Integration Modes

AuthForge supports two integration styles:

- **Mode A: Headless API / SDK (Supabase-style)** — You build your own UI and use our TypeScript or Go SDKs.
- **Mode B: Hosted OpenID Connect (Clerk-style)** — You redirect users to AuthForge's hosted sign-in page.

---

## Mode A: Headless SDK Integration

### TypeScript / React / Next.js / Node

#### Installation
```bash
# From npm (or local path / git)
npm install authforge-client
```

#### Usage
```typescript
import { AuthForgeClient } from "authforge-client";

// Initialize client with your publishable key
const auth = new AuthForgeClient({
  apiKey: "af_test_pk_...",
  baseUrl: "http://localhost:8080", // Or your production domain
});

// 1. Sign Up
const signupSession = await auth.signUp({
  email: "user@example.com",
  password: "secure-password-123",
});
console.log("Access Token:", signupSession.access_token);

// 2. Log In
const session = await auth.logIn({
  email: "user@example.com",
  password: "secure-password-123",
});

// 3. Fetch Authenticated User Profile
const user = await auth.getCurrentUser(session.access_token);
console.log("Logged in user:", user.user_id, "Verified:", user.email_verified);

// 4. Token Refresh (Automatic rotation)
const newSession = await auth.refresh(session.refresh_token);

// 5. Log Out
await auth.logOut({ refreshToken: newSession.refresh_token });
```

---

### Go Backend

#### Installation
```bash
go get github.com/wreckx-in-scene/AuthForge/sdk/go
```

#### Usage
```go
package main

import (
	"context"
	"fmt"
	"log"

	authforge "github.com/wreckx-in-scene/AuthForge/sdk/go"
)

func main() {
	client, err := authforge.NewClient(authforge.Config{
		APIKey:  "af_test_pk_...",
		BaseURL: "http://localhost:8080",
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Sign Up
	tokens, err := client.SignUp(ctx, "developer@example.com", "password123")
	if err != nil {
		log.Fatalf("sign up error: %v", err)
	}

	// Verify User
	user, err := client.GetCurrentUser(ctx, tokens.AccessToken)
	if err != nil {
		log.Fatalf("get user error: %v", err)
	}
	fmt.Printf("Authenticated: %s\n", user.UserID)
}
```

---

## Mode B: Hosted OpenID Connect (OIDC)

AuthForge is a standard OpenID Connect Provider with authorization code flow + PKCE.

1. In the AuthForge Console, go to **OAuth Clients** and register your client:
   - **Name**: `My Web App`
   - **Callback URL**: `http://localhost:3000/auth/callback`
2. Redirect users to the authorization URL:
   ```
   http://localhost:8080/authorize?client_id=<CLIENT_ID>&redirect_uri=http://localhost:3000/auth/callback&response_type=code&scope=openid email&code_challenge=<S256_CHALLENGE>&code_challenge_method=S256&state=<RANDOM_STATE>
   ```
3. Exchange the returned authorization code for an ID token:
   ```bash
   curl -X POST http://localhost:8080/v1/token \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=authorization_code&client_id=<CLIENT_ID>&code=<CODE>&redirect_uri=http://localhost:3000/auth/callback&code_verifier=<CODE_VERIFIER>"
   ```
4. Verify the ID token signature against AuthForge's public JWKS endpoint:
   ```
   http://localhost:8080/.well-known/jwks.json
   ```

---

## Security Features Built-in

- **Argon2id** password hashing.
- **Ed25519 (EdDSA)** JWT asymmetric access tokens.
- **Automatic Refresh Token Rotation** with theft detection (replay revokes the session family).
- **Multi-Tenant Row-Level Security (RLS)** in PostgreSQL.
