package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"

	tenantv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/tenant/v1"
)

type consoleSignupTenantClient struct {
	tenantv1.TenantServiceClient
	created bool
}

func (c *consoleSignupTenantClient) CreateAccount(_ context.Context, _ *tenantv1.CreateAccountRequest, _ ...grpc.CallOption) (*tenantv1.CreateAccountResponse, error) {
	c.created = true
	return &tenantv1.CreateAccountResponse{
		AccessToken: "test-token",
		ExpiresAt:   1,
		Account:     &tenantv1.Account{Id: "account-1", Email: "tester@example.com", Name: "Tester"},
	}, nil
}

func TestConsoleSignupRequiresConfiguredInviteCode(t *testing.T) {
	for _, test := range []struct {
		name       string
		inviteCode string
	}{
		{name: "missing code"},
		{name: "incorrect code", inviteCode: "wrong"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &consoleSignupTenantClient{}
			server := &Server{tenant: client, inviteCode: "beta-secret"}
			body := `{"email":"tester@example.com","password":"password","name":"Tester"}`
			if test.inviteCode != "" {
				body = strings.TrimSuffix(body, "}") + `,"invite_code":"` + test.inviteCode + `"}`
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/console/signup", strings.NewReader(body))
			response := httptest.NewRecorder()

			server.consoleSignup(response, request)

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
			}
			if !strings.Contains(response.Body.String(), "invalid_invite_code") {
				t.Fatalf("response = %q, want invalid_invite_code", response.Body.String())
			}
			if client.created {
				t.Fatal("account was created without the correct invite code")
			}
		})
	}
}

func TestConsoleSignupAcceptsConfiguredInviteCode(t *testing.T) {
	client := &consoleSignupTenantClient{}
	server := &Server{tenant: client, inviteCode: "beta-secret"}
	request := httptest.NewRequest(http.MethodPost, "/v1/console/signup",
		strings.NewReader(`{"email":"tester@example.com","password":"password","name":"Tester","invite_code":"beta-secret"}`))
	response := httptest.NewRecorder()

	server.consoleSignup(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if !client.created {
		t.Fatal("account was not created with the correct invite code")
	}
}
