package authforge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSignUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/signup" {
			t.Errorf("expected /v1/signup, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "test_key" {
			t.Errorf("expected X-Api-Key test_key, got %s", r.Header.Get("X-Api-Key"))
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["email"] != "ada@example.com" || body["password"] != "secret123" {
			t.Errorf("unexpected body: %v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"access_token": "af_at_test",
			"refresh_token": "af_rt_test",
			"expires_at": 1700000000,
			"user_id": "user-uuid-1",
			"session_id": "session-uuid-1",
			"email_verified": false
		}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test_key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tokens, err := client.SignUp(context.Background(), "ada@example.com", "secret123")
	if err != nil {
		t.Fatalf("SignUp failed: %v", err)
	}

	if tokens.AccessToken != "af_at_test" {
		t.Errorf("expected access_token af_at_test, got %s", tokens.AccessToken)
	}
	if tokens.RefreshToken != "af_rt_test" {
		t.Errorf("expected refresh_token af_rt_test, got %s", tokens.RefreshToken)
	}
	if tokens.UserID != "user-uuid-1" {
		t.Errorf("expected user_id user-uuid-1, got %s", tokens.UserID)
	}
	if tokens.EmailVerified {
		t.Errorf("expected email_verified false")
	}
}

func TestRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/token/refresh" {
			t.Errorf("expected /v1/token/refresh, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "" {
			t.Errorf("refresh must not send X-Api-Key")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "af_at_new",
			"refresh_token": "af_rt_new",
			"expires_at": 1700001000,
			"user_id": "user-uuid-1",
			"session_id": "session-uuid-1",
			"email_verified": true
		}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{APIKey: "test_key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tokens, err := client.Refresh(context.Background(), "af_rt_old")
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if tokens.AccessToken != "af_at_new" || tokens.RefreshToken != "af_rt_new" {
		t.Errorf("unexpected tokens: %v", tokens)
	}
	if !tokens.EmailVerified {
		t.Errorf("expected email_verified true")
	}
}

func TestGetCurrentUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/me" {
			t.Errorf("expected /v1/me, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer valid_jwt_token" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"user_id": "usr_123",
			"environment_id": "env_456",
			"project_id": "prj_789",
			"session_id": "ses_000",
			"email_verified": true
		}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{APIKey: "test_key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	user, err := client.GetCurrentUser(context.Background(), "valid_jwt_token")
	if err != nil {
		t.Fatalf("GetCurrentUser failed: %v", err)
	}
	if user.UserID != "usr_123" || user.ProjectID != "prj_789" {
		t.Errorf("unexpected user: %v", user)
	}
}

func TestRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{APIKey: "test_key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	_, err = client.LogIn(context.Background(), "a@b.com", "pass")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var apiErr *Error
	if !isError(err, &apiErr) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", apiErr.StatusCode)
	}
	if apiErr.Code != "rate_limited" {
		t.Errorf("expected rate_limited, got %s", apiErr.Code)
	}
	if apiErr.RetryAfter != 12 {
		t.Errorf("expected RetryAfter 12, got %d", apiErr.RetryAfter)
	}
}

func isError(err error, target **Error) bool {
	if e, ok := err.(*Error); ok {
		*target = e
		return true
	}
	return false
}
