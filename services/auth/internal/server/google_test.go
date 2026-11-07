package server

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

func TestVerifyGoogleCredential(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier := oidc.NewVerifier("https://accounts.google.com", &oidc.StaticKeySet{
		PublicKeys: []crypto.PublicKey{&key.PublicKey},
	}, &oidc.Config{ClientID: "authforge-client"})
	now := time.Now()

	tests := []struct {
		name        string
		issuer      string
		audience    string
		nonce       string
		email       string
		verified    bool
		expiry      time.Time
		signingKey  *rsa.PrivateKey
		wantSuccess bool
	}{
		{
			name: "valid credential", issuer: "https://accounts.google.com",
			audience: "authforge-client", nonce: "nonce-1", email: "ada@example.com",
			verified: true, expiry: now.Add(time.Minute), signingKey: key, wantSuccess: true,
		},
		{
			name: "nonce mismatch", issuer: "https://accounts.google.com",
			audience: "authforge-client", nonce: "other-nonce", email: "ada@example.com",
			verified: true, expiry: now.Add(time.Minute), signingKey: key,
		},
		{
			name: "unverified email", issuer: "https://accounts.google.com",
			audience: "authforge-client", nonce: "nonce-1", email: "ada@example.com",
			verified: false, expiry: now.Add(time.Minute), signingKey: key,
		},
		{
			name: "wrong audience", issuer: "https://accounts.google.com",
			audience: "other-client", nonce: "nonce-1", email: "ada@example.com",
			verified: true, expiry: now.Add(time.Minute), signingKey: key,
		},
		{
			name: "wrong issuer", issuer: "https://attacker.example",
			audience: "authforge-client", nonce: "nonce-1", email: "ada@example.com",
			verified: true, expiry: now.Add(time.Minute), signingKey: key,
		},
		{
			name: "expired token", issuer: "https://accounts.google.com",
			audience: "authforge-client", nonce: "nonce-1", email: "ada@example.com",
			verified: true, expiry: now.Add(-time.Minute), signingKey: key,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := signGoogleToken(t, tt.signingKey, tt.issuer, tt.audience, tt.nonce, tt.email, tt.verified, tt.expiry)
			claims, err := verifyGoogleCredential(context.Background(), verifier, raw, "nonce-1")
			if tt.wantSuccess {
				if err != nil {
					t.Fatal(err)
				}
				if claims.Subject != "google-subject" || claims.Email != tt.email || !claims.EmailVerified {
					t.Fatalf("unexpected claims: %+v", claims)
				}
			} else if err == nil {
				t.Fatal("expected credential validation to fail")
			}
		})
	}

	t.Run("invalid signature", func(t *testing.T) {
		wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		raw := signGoogleToken(t, wrongKey, "https://accounts.google.com", "authforge-client", "nonce-1", "ada@example.com", true, now.Add(time.Minute))
		if _, err := verifyGoogleCredential(context.Background(), verifier, raw, "nonce-1"); err == nil {
			t.Fatal("expected invalid signature to fail")
		}
	})
}

type googleTestClaims struct {
	jwt.RegisteredClaims
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Nonce         string `json:"nonce"`
}

func signGoogleToken(t *testing.T, key *rsa.PrivateKey, issuer, audience, nonce, email string, verified bool, expiry time.Time) string {
	t.Helper()
	now := time.Now()
	claims := googleTestClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   "google-subject",
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email:         email,
		EmailVerified: verified,
		Nonce:         nonce,
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
