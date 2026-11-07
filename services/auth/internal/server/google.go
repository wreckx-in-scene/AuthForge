package server

import (
	"context"
	"errors"

	"github.com/coreos/go-oidc/v3/oidc"
)

type googleClaims struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Nonce         string `json:"nonce"`
}

func verifyGoogleCredential(ctx context.Context, verifier *oidc.IDTokenVerifier, rawToken, expectedNonce string) (googleClaims, error) {
	if verifier == nil || rawToken == "" || expectedNonce == "" {
		return googleClaims{}, errors.New("Google credential is incomplete")
	}
	idToken, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return googleClaims{}, errors.New("Google ID token verification failed")
	}
	var claims googleClaims
	if err := idToken.Claims(&claims); err != nil {
		return googleClaims{}, errors.New("Google ID token claims are invalid")
	}
	if claims.Subject == "" || claims.Email == "" || !claims.EmailVerified || claims.Nonce != expectedNonce {
		return googleClaims{}, errors.New("Google identity claims are incomplete or untrusted")
	}
	return claims, nil
}
