package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	tenantv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/tenant/v1"
)

func TestOpenIDConfiguration(t *testing.T) {
	const issuer = "https://auth.example.com"
	server := &Server{issuer: issuer}
	response := httptest.NewRecorder()
	server.openIDConfiguration(response, httptest.NewRequest("GET", "/.well-known/openid-configuration", nil))

	if response.Code != 200 {
		t.Fatalf("openIDConfiguration() status = %d, want 200", response.Code)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("openIDConfiguration() did not allow public cross-origin access")
	}
	var metadata struct {
		Issuer                 string   `json:"issuer"`
		AuthorizationEndpoint  string   `json:"authorization_endpoint"`
		TokenEndpoint          string   `json:"token_endpoint"`
		JWKSURI                string   `json:"jwks_uri"`
		ResponseTypesSupported []string `json:"response_types_supported"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &metadata); err != nil {
		t.Fatalf("decode discovery metadata: %v", err)
	}
	if metadata.Issuer != issuer ||
		metadata.AuthorizationEndpoint != issuer+"/authorize" ||
		metadata.TokenEndpoint != issuer+"/v1/token" ||
		metadata.JWKSURI != issuer+"/.well-known/jwks.json" {
		t.Fatalf("discovery metadata has inconsistent endpoints: %+v", metadata)
	}
	if len(metadata.ResponseTypesSupported) != 1 || metadata.ResponseTypesSupported[0] != "code" {
		t.Fatalf("response_types_supported = %v, want [code]", metadata.ResponseTypesSupported)
	}
}

func TestSupportedOIDCScope(t *testing.T) {
	for _, scope := range []string{"openid email", "email openid"} {
		if !supportedOIDCScope(scope) {
			t.Errorf("supportedOIDCScope(%q) = false", scope)
		}
	}
	for _, scope := range []string{"openid", "openid profile", "openid email profile", "email"} {
		if supportedOIDCScope(scope) {
			t.Errorf("supportedOIDCScope(%q) = true", scope)
		}
	}
}

func TestValidS256Challenge(t *testing.T) {
	const challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if !validS256Challenge(challenge) {
		t.Fatal("validS256Challenge() rejected a valid S256 challenge")
	}
	for _, invalid := range []string{"", challenge + "=", "not-a-challenge"} {
		if validS256Challenge(invalid) {
			t.Errorf("validS256Challenge(%q) = true", invalid)
		}
	}
}

func TestRegisteredOrigin(t *testing.T) {
	client := &tenantv1.OAuthClient{
		RedirectUris: []string{
			"https://app.example.com/auth/callback",
			"http://localhost:3000/callback",
		},
	}
	for _, origin := range []string{"https://app.example.com", "http://localhost:3000"} {
		if !registeredOrigin(client, origin) {
			t.Errorf("registeredOrigin() rejected %q", origin)
		}
	}
	for _, origin := range []string{
		"https://other.example.com", "http://localhost:3001",
		"https://app.example.com/path", "null",
	} {
		if registeredOrigin(client, origin) {
			t.Errorf("registeredOrigin() accepted %q", origin)
		}
	}
}
