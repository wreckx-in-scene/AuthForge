package token

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(private.Seed(), "http://localhost:8080", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := signer.SignAccess(
		"6d5b9c1e-1b2a-4c3d-8e9f-001122334455",
		"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"11111111-2222-3333-4444-555555555555",
		"99999999-8888-7777-6666-555555555555",
		false,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := signer.Verifier().ParseAccess(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "6d5b9c1e-1b2a-4c3d-8e9f-001122334455" || claims.Audience[0] != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" || claims.EmailVerified {
		t.Fatalf("claims: %+v", claims)
	}

	console, _, err := SignConsole([]byte("0123456789abcdef0123456789abcdef"), "http://localhost:8080", claims.Subject, time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signer.Verifier().ParseAccess(console); err == nil {
		t.Fatal("console token was accepted as an access token")
	}
}

func TestExpiredAccessToken(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(private.Seed(), "http://localhost:8080", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := signer.SignAccess("6d5b9c1e-1b2a-4c3d-8e9f-001122334455", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "11111111-2222-3333-4444-555555555555", "99999999-8888-7777-6666-555555555555", false, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signer.Verifier().ParseAccess(raw); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

func TestRefreshTokenPrefix(t *testing.T) {
	plain, sum, err := NewRefresh()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, "af_rt_") || len(sum) != 32 {
		t.Fatalf("plain=%s sum=%d", plain, len(sum))
	}
}

func TestIDTokenClaims(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := NewSigner(private.Seed(), "https://auth.example.com", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := signer.SignIDToken(
		"6d5b9c1e-1b2a-4c3d-8e9f-001122334455",
		"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"ada@example.com", "client-nonce", false, time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jwt.ParseWithClaims(raw, &IDTokenClaims{}, func(_ *jwt.Token) (any, error) {
		return signer.public, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer("https://auth.example.com"),
		jwt.WithAudience("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		t.Fatal(err)
	}
	claims, ok := parsed.Claims.(*IDTokenClaims)
	if !ok || !parsed.Valid {
		t.Fatal("ID token claims are invalid")
	}
	if claims.Subject != "6d5b9c1e-1b2a-4c3d-8e9f-001122334455" ||
		claims.Email != "ada@example.com" || claims.EmailVerified || claims.Nonce != "client-nonce" {
		t.Fatalf("unexpected ID token claims: %+v", claims)
	}
}
