# AuthForge Go SDK

An idiomatic, zero-dependency Go client library for the AuthForge application authentication API.

## Installation

```bash
go get github.com/wreckx-in-scene/AuthForge/sdk/go
```

## Quick Start

### Initialize Client

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
		APIKey:  "af_test_pk_...",           // Your environment publishable API key
		BaseURL: "http://localhost:8080",     // Or your production AuthForge domain
	})
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// 1. Sign up a new user
	tokens, err := client.SignUp(ctx, "ada@example.com", "password123")
	if err != nil {
		log.Fatalf("sign up failed: %v", err)
	}
	fmt.Printf("User created: %s, Access Token: %s\n", tokens.UserID, tokens.AccessToken)

	// 2. Fetch current user from access token
	user, err := client.GetCurrentUser(ctx, tokens.AccessToken)
	if err != nil {
		log.Fatalf("get user failed: %v", err)
	}
	fmt.Printf("Authenticated User ID: %s, Verified: %v\n", user.UserID, user.EmailVerified)

	// 3. Refresh token (automatic rotation)
	newTokens, err := client.Refresh(ctx, tokens.RefreshToken)
	if err != nil {
		log.Fatalf("refresh failed: %v", err)
	}
	fmt.Printf("Rotated Refresh Token: %s\n", newTokens.RefreshToken)

	// 4. Log out (revokes session family)
	_, err = client.LogOut(ctx, newTokens.RefreshToken, "")
	if err != nil {
		log.Fatalf("logout failed: %v", err)
	}
	fmt.Println("Session successfully revoked.")
}
```

## Features

- **Standard Library Only**: Requires no external dependencies.
- **Context Aware**: All network operations accept a `context.Context` for deadlines and cancellation.
- **Error Types**: Returns typed `*authforge.Error` with HTTP status code and `RetryAfter` for rate-limiting handling.
- **Full API Coverage**:
  - `SignUp(ctx, email, password)`
  - `LogIn(ctx, email, password)`
  - `Refresh(ctx, refreshToken)`
  - `LogOut(ctx, refreshToken, accessToken)`
  - `GetCurrentUser(ctx, accessToken)`
  - `RequestEmailVerification(ctx, email)`
  - `VerifyEmail(ctx, environmentID, token)`
  - `RequestGoogleNonce(ctx)`
  - `SignInWithGoogle(ctx, idToken, nonce)`
