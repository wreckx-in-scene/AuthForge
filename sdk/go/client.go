package authforge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Config provides configuration options for the AuthForge Client.
type Config struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// Client is a Go client for AuthForge's application authentication API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// AuthTokens contains the tokens and identity information returned upon sign-up, login, or refresh.
type AuthTokens struct {
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token"`
	ExpiresAt     int64  `json:"expires_at"`
	UserID        string `json:"user_id"`
	SessionID     string `json:"session_id"`
	EmailVerified bool   `json:"email_verified"`
}

// CurrentUser contains the user and environment identity claims from an access token.
type CurrentUser struct {
	UserID        string `json:"user_id"`
	EnvironmentID string `json:"environment_id"`
	ProjectID     string `json:"project_id"`
	SessionID     string `json:"session_id"`
	EmailVerified bool   `json:"email_verified"`
}

// LogoutResult contains the identifier of the revoked session.
type LogoutResult struct {
	SessionID string `json:"session_id"`
}

// Error represents an error returned by the AuthForge API.
type Error struct {
	StatusCode int
	Code       string
	RetryAfter int
}

func (e *Error) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("authforge: API error %s (status %d, retry after %ds)", e.Code, e.StatusCode, e.RetryAfter)
	}
	return fmt.Sprintf("authforge: API error %s (status %d)", e.Code, e.StatusCode)
}

// NewClient creates a new AuthForge Client instance with the given configuration.
func NewClient(cfg Config) (*Client, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("authforge: API key is required")
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	return &Client{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}, nil
}

// SignUp registers a new application user with email and password.
func (c *Client) SignUp(ctx context.Context, email, password string) (*AuthTokens, error) {
	return c.requestTokens(ctx, "/v1/signup", map[string]string{
		"email":    email,
		"password": password,
	}, true, "")
}

// LogIn authenticates an existing user with email and password.
func (c *Client) LogIn(ctx context.Context, email, password string) (*AuthTokens, error) {
	return c.requestTokens(ctx, "/v1/login", map[string]string{
		"email":    email,
		"password": password,
	}, true, "")
}

// Refresh rotates a refresh token and returns a new access token and refresh token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*AuthTokens, error) {
	return c.requestTokens(ctx, "/v1/token/refresh", map[string]string{
		"refresh_token": refreshToken,
	}, false, "")
}

// RequestGoogleNonce requests a one-time nonce to initiate Google Identity Services sign-in.
func (c *Client) RequestGoogleNonce(ctx context.Context) (string, error) {
	var resp struct {
		Nonce string `json:"nonce"`
	}
	if err := c.request(ctx, http.MethodPost, "/v1/oauth/google/nonce", nil, true, "", &resp); err != nil {
		return "", err
	}
	if resp.Nonce == "" {
		return "", &Error{StatusCode: http.StatusOK, Code: "invalid_response"}
	}
	return resp.Nonce, nil
}

// SignInWithGoogle exchanges a Google ID token and one-time nonce for AuthForge session tokens.
func (c *Client) SignInWithGoogle(ctx context.Context, idToken, nonce string) (*AuthTokens, error) {
	return c.requestTokens(ctx, "/v1/oauth/google", map[string]string{
		"id_token": idToken,
		"nonce":    nonce,
	}, true, "")
}

// RequestEmailVerification triggers an email verification message to be sent to the specified address.
func (c *Client) RequestEmailVerification(ctx context.Context, email string) error {
	var resp struct {
		Status string `json:"status"`
	}
	if err := c.request(ctx, http.MethodPost, "/v1/email/verification/request", map[string]string{
		"email": email,
	}, true, "", &resp); err != nil {
		return err
	}
	if resp.Status != "ok" {
		return &Error{StatusCode: http.StatusOK, Code: "invalid_response"}
	}
	return nil
}

// VerifyEmail confirms an email verification token for a specific environment.
func (c *Client) VerifyEmail(ctx context.Context, environmentID, token string) (bool, error) {
	var resp struct {
		Verified bool `json:"verified"`
	}
	if err := c.request(ctx, http.MethodPost, "/v1/email/verification/confirm", map[string]string{
		"environment_id": environmentID,
		"token":          token,
	}, false, "", &resp); err != nil {
		return false, err
	}
	return resp.Verified, nil
}

// LogOut revokes an active session using either a refresh token or an access token.
func (c *Client) LogOut(ctx context.Context, refreshToken, accessToken string) (*LogoutResult, error) {
	if refreshToken == "" && accessToken == "" {
		return nil, errors.New("authforge: a refresh token or access token is required to log out")
	}

	var body map[string]string
	bearer := ""
	if refreshToken != "" {
		body = map[string]string{"refresh_token": refreshToken}
	} else {
		bearer = accessToken
	}

	var resp LogoutResult
	if err := c.request(ctx, http.MethodPost, "/v1/logout", body, false, bearer, &resp); err != nil {
		return nil, err
	}
	if resp.SessionID == "" {
		return nil, &Error{StatusCode: http.StatusOK, Code: "invalid_response"}
	}
	return &resp, nil
}

// GetCurrentUser returns the claims associated with a valid access token.
func (c *Client) GetCurrentUser(ctx context.Context, accessToken string) (*CurrentUser, error) {
	if accessToken == "" {
		return nil, errors.New("authforge: access token is required")
	}

	var resp CurrentUser
	if err := c.request(ctx, http.MethodGet, "/v1/me", nil, false, accessToken, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) requestTokens(ctx context.Context, path string, body any, includeAPIKey bool, bearer string) (*AuthTokens, error) {
	var tokens AuthTokens
	if err := c.request(ctx, http.MethodPost, path, body, includeAPIKey, bearer, &tokens); err != nil {
		return nil, err
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.UserID == "" {
		return nil, &Error{StatusCode: http.StatusOK, Code: "invalid_response"}
	}
	return &tokens, nil
}

func (c *Client) request(ctx context.Context, method, path string, body any, includeAPIKey bool, bearer string, target any) error {
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("authforge: failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("authforge: failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if includeAPIKey {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("authforge: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("authforge: failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errPayload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBytes, &errPayload)
		code := errPayload.Error
		if code == "" {
			code = "request_failed"
		}

		retryAfter := 0
		if rawRetry := resp.Header.Get("Retry-After"); rawRetry != "" {
			if sec, parseErr := strconv.Atoi(strings.TrimSpace(rawRetry)); parseErr == nil && sec >= 0 {
				retryAfter = sec
			}
		}

		return &Error{
			StatusCode: resp.StatusCode,
			Code:       code,
			RetryAfter: retryAfter,
		}
	}

	if target != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, target); err != nil {
			return &Error{
				StatusCode: resp.StatusCode,
				Code:       "invalid_response",
			}
		}
	}

	return nil
}
