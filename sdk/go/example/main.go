package main

import (
	"context"
	"fmt"
	"log"

	authforge "github.com/wreckx-in-scene/AuthForge/sdk/go"
)

func main() {
	client, err := authforge.NewClient(authforge.Config{
		APIKey:  "af_test_pk_r-dPgY0zaqBEHLeia1Y6Y2l3rdxExnxJ67bKOM3MLus",
		BaseURL: "http://localhost:8080",
	})
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// 1. Sign up
	email := "alex@student.campus.edu"
	tokens, err := client.SignUp(ctx, email, "SuperSecurePassword123!")
	if err != nil {
		log.Fatalf("SignUp failed: %v", err)
	}
	fmt.Printf("[1] SignUp SUCCESS -> UserID: %s\n", tokens.UserID)

	// 2. Get Current User
	user, err := client.GetCurrentUser(ctx, tokens.AccessToken)
	if err != nil {
		log.Fatalf("GetCurrentUser failed: %v", err)
	}
	fmt.Printf("[2] GetCurrentUser SUCCESS -> UserID: %s, ProjectID: %s, EnvID: %s\n", user.UserID, user.ProjectID, user.EnvironmentID)

	// 3. Refresh Token
	rotated, err := client.Refresh(ctx, tokens.RefreshToken)
	if err != nil {
		log.Fatalf("Refresh failed: %v", err)
	}
	fmt.Printf("[3] Refresh SUCCESS -> New Token Expiry: %d\n", rotated.ExpiresAt)

	// 4. Log out
	logoutRes, err := client.LogOut(ctx, rotated.RefreshToken, "")
	if err != nil {
		log.Fatalf("LogOut failed: %v", err)
	}
	fmt.Printf("[4] LogOut SUCCESS -> Revoked SessionID: %s\n", logoutRes.SessionID)

	fmt.Println("\nAll Go SDK live operations verified successfully!")
}
