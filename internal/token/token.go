package token

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/wreckx-in-scene/AuthForge/internal/digest"
	"github.com/wreckx-in-scene/AuthForge/internal/errs"
)

const (
	accessKID       = "af-ed25519-1"
	ConsoleAudience = "authforge-console"
)

type AccessClaims struct {
	jwt.RegisteredClaims
	SID           string `json:"sid"`
	ProjectID     string `json:"pid"`
	EmailVerified bool   `json:"email_verified"`
}

type Signer struct {
	issuer  string
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	ttl     time.Duration
}

type Verifier struct {
	issuer  string
	publics map[string]ed25519.PublicKey
}

func NewSigner(seed []byte, issuer string, ttl time.Duration) (*Signer, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing seed must be 32 bytes")
	}
	if issuer == "" || ttl <= 0 {
		return nil, fmt.Errorf("issuer and access ttl are required")
	}
	private := ed25519.NewKeyFromSeed(seed)
	public, ok := private.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("ed25519 public key")
	}
	return &Signer{issuer: issuer, private: private, public: public, ttl: ttl}, nil
}

func ParseSeed(encoded string) ([]byte, error) {
	seed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode signing seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing seed must be 32 bytes")
	}
	return seed, nil
}

func (s *Signer) KID() string { return accessKID }

func (s *Signer) PublicKey() []byte {
	out := make([]byte, len(s.public))
	copy(out, s.public)
	return out
}

func (s *Signer) Verifier() *Verifier {
	return &Verifier{issuer: s.issuer, publics: map[string]ed25519.PublicKey{accessKID: s.public}}
}

func NewVerifier(kid string, public []byte, issuer string) (*Verifier, error) {
	if kid == "" || issuer == "" || len(public) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("verification key is incomplete")
	}
	return &Verifier{issuer: issuer, publics: map[string]ed25519.PublicKey{kid: ed25519.PublicKey(public)}}, nil
}

func (s *Signer) SignAccess(userID, environmentID, projectID, sessionID string, emailVerified bool, now time.Time) (string, time.Time, error) {
	expires := now.Add(s.ttl)
	claims := AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{environmentID},
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
		SID:           sessionID,
		ProjectID:     projectID,
		EmailVerified: emailVerified,
	}
	unsigned := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	unsigned.Header["kid"] = accessKID
	signed, err := unsigned.SignedString(s.private)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

func (v *Verifier) ParseAccess(raw string) (*AccessClaims, error) {
	parsed, err := jwt.ParseWithClaims(raw, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		public, ok := v.publics[kid]
		if !ok {
			return nil, errs.ErrUnauthenticated
		}
		return public, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}), jwt.WithIssuer(v.issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, errs.ErrUnauthenticated
	}
	claims, ok := parsed.Claims.(*AccessClaims)
	if !ok || claims.Subject == "" || claims.SID == "" || claims.ProjectID == "" || len(claims.Audience) != 1 || claims.Audience[0] == "" {
		return nil, errs.ErrUnauthenticated
	}
	return claims, nil
}

func SignConsole(secret []byte, issuer, accountID string, ttl time.Duration, now time.Time) (string, time.Time, error) {
	if len(secret) < 32 || issuer == "" || ttl <= 0 {
		return "", time.Time{}, errors.New("console signer is not configured")
	}
	expires := now.Add(ttl)
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   accountID,
		Audience:  jwt.ClaimStrings{ConsoleAudience},
		ExpiresAt: jwt.NewNumericDate(expires),
		IssuedAt:  jwt.NewNumericDate(now),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	return signed, expires, err
}

func ParseConsole(secret []byte, issuer, raw string) (string, error) {
	parsed, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(issuer), jwt.WithAudience(ConsoleAudience), jwt.WithExpirationRequired())
	if err != nil {
		return "", errs.ErrUnauthenticated
	}
	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok || claims.Subject == "" {
		return "", errs.ErrUnauthenticated
	}
	if _, err := uuid.Parse(claims.Subject); err != nil {
		return "", errs.ErrUnauthenticated
	}
	return claims.Subject, nil
}

func NewRefresh() (plain string, sum []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	plain = "af_rt_" + base64.RawURLEncoding.EncodeToString(raw)
	return plain, digest.SHA256(plain), nil
}

func Bearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	value := strings.TrimSpace(header[len(prefix):])
	if value == "" {
		return "", false
	}
	return value, true
}
