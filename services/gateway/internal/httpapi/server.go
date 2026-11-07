package httpapi

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/auth/v1"
	tenantv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/tenant/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/token"
	"github.com/wreckx-in-scene/AuthForge/services/gateway/internal/dashboard"
)

type Server struct {
	tenant        tenantv1.TenantServiceClient
	auth          authv1.AuthServiceClient
	verifier      *token.Verifier
	consoleSecret []byte
	issuer        string
	log           *slog.Logger
	rateLimiter   *rateLimiter
}

func New(tenant tenantv1.TenantServiceClient, auth authv1.AuthServiceClient, verifier *token.Verifier, consoleSecret []byte, issuer string, log *slog.Logger) *Server {
	return &Server{
		tenant: tenant, auth: auth, verifier: verifier, consoleSecret: consoleSecret,
		issuer: issuer, log: log, rateLimiter: newRateLimiter(),
	}
}

func LoadVerifier(ctx context.Context, authClient authv1.AuthServiceClient, issuer string) (*token.Verifier, error) {
	deadline := time.Now().Add(30 * time.Second)
	var last error
	for {
		resp, err := authClient.VerificationKeys(ctx, &authv1.VerificationKeysRequest{})
		if err == nil && resp.GetAlg() == "EdDSA" && len(resp.GetPublicKey()) == ed25519.PublicKeySize {
			return token.NewVerifier(resp.GetKid(), resp.GetPublicKey(), issuer)
		}
		last = err
		if time.Now().After(deadline) {
			if last == nil {
				last = errors.New("verification key was empty")
			}
			return nil, last
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /{$}", dashboard.Handler())
	mux.Handle("GET /assets/", dashboard.Handler())
	mux.HandleFunc("POST /v1/console/signup", s.consoleSignup)
	mux.HandleFunc("POST /v1/console/login", s.consoleLogin)
	mux.HandleFunc("POST /v1/password-reset/request", s.requestPasswordReset)
	mux.HandleFunc("POST /v1/password-reset/confirm", s.confirmPasswordReset)
	mux.HandleFunc("POST /v1/organizations", s.createOrganization)
	mux.HandleFunc("POST /v1/projects", s.createProject)
	mux.HandleFunc("POST /v1/environments", s.createEnvironment)
	mux.HandleFunc("POST /v1/api-keys", s.issueAPIKey)
	mux.HandleFunc("POST /v1/signup", s.signup)
	mux.HandleFunc("POST /v1/login", s.login)
	mux.HandleFunc("POST /v1/oauth/google/nonce", s.googleNonce)
	mux.HandleFunc("POST /v1/oauth/google", s.googleLogin)
	mux.HandleFunc("POST /v1/email/verification/request", s.requestEmailVerification)
	mux.HandleFunc("POST /v1/email/verification/confirm", s.verifyEmail)
	mux.HandleFunc("POST /v1/token/refresh", s.refresh)
	mux.HandleFunc("POST /v1/logout", s.logout)
	mux.HandleFunc("GET /v1/me", s.me)
	return rateLimitMiddleware(mux, s.rateLimiter)
}

func (s *Server) consoleSignup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.tenant.CreateAccount(ctx, &tenantv1.CreateAccountRequest{Email: body.Email, Password: body.Password, Name: body.Name})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"access_token": resp.GetAccessToken(),
		"expires_at":   resp.GetExpiresAt(),
		"account":      accountJSON(resp.GetAccount()),
	})
}

func (s *Server) consoleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.tenant.LoginAccount(ctx, &tenantv1.LoginAccountRequest{Email: body.Email, Password: body.Password})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": resp.GetAccessToken(),
		"expires_at":   resp.GetExpiresAt(),
		"account":      accountJSON(resp.GetAccount()),
	})
}

func (s *Server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	_, err := s.tenant.RequestPasswordReset(ctx, &tenantv1.RequestPasswordResetRequest{Email: body.Email})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) confirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ResetToken string `json:"reset_token"`
		Password   string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.tenant.ResetPassword(ctx, &tenantv1.ResetPasswordRequest{ResetToken: body.ResetToken, Password: body.Password})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": resp.GetOk()})
}

func (s *Server) createOrganization(w http.ResponseWriter, r *http.Request) {
	accountID, ok := s.consoleAccount(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.tenant.CreateOrganization(ctx, &tenantv1.CreateOrganizationRequest{AccountId: accountID, Name: body.Name, Slug: body.Slug})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": resp.GetId(), "name": resp.GetName(), "slug": resp.GetSlug()})
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	accountID, ok := s.consoleAccount(w, r)
	if !ok {
		return
	}
	var body struct {
		OrganizationID string `json:"organization_id"`
		Name           string `json:"name"`
		Slug           string `json:"slug"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.tenant.CreateProject(ctx, &tenantv1.CreateProjectRequest{
		AccountId:      accountID,
		OrganizationId: body.OrganizationID,
		Name:           body.Name,
		Slug:           body.Slug,
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":              resp.GetId(),
		"organization_id": resp.GetOrganizationId(),
		"name":            resp.GetName(),
		"slug":            resp.GetSlug(),
	})
}

func (s *Server) createEnvironment(w http.ResponseWriter, r *http.Request) {
	accountID, ok := s.consoleAccount(w, r)
	if !ok {
		return
	}
	var body struct {
		ProjectID string `json:"project_id"`
		Name      string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.tenant.CreateEnvironment(ctx, &tenantv1.CreateEnvironmentRequest{
		AccountId: accountID,
		ProjectId: body.ProjectID,
		Name:      body.Name,
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":              resp.GetId(),
		"project_id":      resp.GetProjectId(),
		"organization_id": resp.GetOrganizationId(),
		"name":            resp.GetName(),
	})
}

func (s *Server) issueAPIKey(w http.ResponseWriter, r *http.Request) {
	accountID, ok := s.consoleAccount(w, r)
	if !ok {
		return
	}
	var body struct {
		EnvironmentID string `json:"environment_id"`
		KeyClass      string `json:"key_class"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.tenant.IssueAPIKey(ctx, &tenantv1.IssueAPIKeyRequest{
		AccountId:     accountID,
		EnvironmentId: body.EnvironmentID,
		KeyClass:      body.KeyClass,
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":             resp.GetId(),
		"environment_id": resp.GetEnvironmentId(),
		"key":            resp.GetKey(),
		"key_class":      resp.GetKeyClass(),
		"prefix":         resp.GetPrefix(),
	})
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	s.passwordAuth(w, r, true)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	s.passwordAuth(w, r, false)
}

func (s *Server) googleNonce(w http.ResponseWriter, r *http.Request) {
	resolved, ok := s.resolveKey(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.auth.CreateGoogleNonce(ctx, &authv1.CreateGoogleNonceRequest{
		EnvironmentId: resolved.GetEnvironmentId(),
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"nonce": resp.GetNonce()})
}

func (s *Server) googleLogin(w http.ResponseWriter, r *http.Request) {
	resolved, ok := s.resolveKey(w, r)
	if !ok {
		return
	}
	var body struct {
		IDToken string `json:"id_token"`
		Nonce   string `json:"nonce"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	resp, err := s.auth.GoogleLogin(ctx, &authv1.GoogleLoginRequest{
		EnvironmentId: resolved.GetEnvironmentId(),
		ProjectId:     resolved.GetProjectId(),
		IdToken:       body.IDToken,
		Nonce:         body.Nonce,
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeTokens(w, http.StatusOK, resp.GetAccessToken(), resp.GetRefreshToken(), resp.GetAccessExpiresAt(), resp.GetUserId(), resp.GetSessionId(), resp.GetEmailVerified())
}

func (s *Server) requestEmailVerification(w http.ResponseWriter, r *http.Request) {
	resolved, ok := s.resolveKey(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	_, err := s.auth.RequestEmailVerification(ctx, &authv1.RequestEmailVerificationRequest{
		EnvironmentId: resolved.GetEnvironmentId(),
		Email:         body.Email,
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EnvironmentID string `json:"environment_id"`
		Token         string `json:"token"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.auth.VerifyEmail(ctx, &authv1.VerifyEmailRequest{
		EnvironmentId: body.EnvironmentID,
		Token:         body.Token,
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"verified": resp.GetVerified()})
}

func (s *Server) passwordAuth(w http.ResponseWriter, r *http.Request, signup bool) {
	resolved, ok := s.resolveKey(w, r)
	if !ok {
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if signup {
		resp, err := s.auth.SignUp(ctx, &authv1.SignUpRequest{
			EnvironmentId: resolved.GetEnvironmentId(),
			ProjectId:     resolved.GetProjectId(),
			Email:         body.Email,
			Password:      body.Password,
		})
		if err != nil {
			writeStatus(w, err)
			return
		}
		writeTokens(w, http.StatusCreated, resp.GetAccessToken(), resp.GetRefreshToken(), resp.GetAccessExpiresAt(), resp.GetUserId(), resp.GetSessionId(), resp.GetEmailVerified())
		return
	}
	resp, err := s.auth.Login(ctx, &authv1.LoginRequest{
		EnvironmentId: resolved.GetEnvironmentId(),
		ProjectId:     resolved.GetProjectId(),
		Email:         body.Email,
		Password:      body.Password,
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeTokens(w, http.StatusOK, resp.GetAccessToken(), resp.GetRefreshToken(), resp.GetAccessExpiresAt(), resp.GetUserId(), resp.GetSessionId(), resp.GetEmailVerified())
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.auth.Refresh(ctx, &authv1.RefreshRequest{RefreshToken: body.RefreshToken})
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeTokens(w, http.StatusOK, resp.GetAccessToken(), resp.GetRefreshToken(), resp.GetAccessExpiresAt(), resp.GetUserId(), resp.GetSessionId(), resp.GetEmailVerified())
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	req := &authv1.LogoutRequest{RefreshToken: body.RefreshToken}
	if body.RefreshToken == "" {
		raw, ok := token.Bearer(r.Header.Get("Authorization"))
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		claims, err := s.verifier.ParseAccess(raw)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		req.SessionId = claims.SID
		if len(claims.Audience) == 1 {
			req.EnvironmentId = claims.Audience[0]
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	resp, err := s.auth.Logout(ctx, req)
	if err != nil {
		writeStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session_id": resp.GetSessionId()})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	raw, ok := token.Bearer(r.Header.Get("Authorization"))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	claims, err := s.verifier.ParseAccess(raw)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":        claims.Subject,
		"environment_id": claims.Audience[0],
		"project_id":     claims.ProjectID,
		"session_id":     claims.SID,
		"email_verified": claims.EmailVerified,
	})
}

func (s *Server) consoleAccount(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw, ok := token.Bearer(r.Header.Get("Authorization"))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return "", false
	}
	accountID, err := token.ParseConsole(s.consoleSecret, s.issuer, raw)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return "", false
	}
	return accountID, true
}

func (s *Server) resolveKey(w http.ResponseWriter, r *http.Request) (*tenantv1.ResolveAPIKeyResponse, bool) {
	key := r.Header.Get("X-Api-Key")
	if key == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_api_key"})
		return nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	resolved, err := s.tenant.ResolveAPIKey(ctx, &tenantv1.ResolveAPIKeyRequest{Key: key})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_api_key"})
			return nil, false
		}
		writeStatus(w, err)
		return nil, false
	}
	return resolved, true
}

func accountJSON(account *tenantv1.Account) map[string]string {
	if account == nil {
		return map[string]string{}
	}
	return map[string]string{"id": account.GetId(), "email": account.GetEmail(), "name": account.GetName()}
}

func writeTokens(w http.ResponseWriter, code int, access, refresh string, exp int64, userID, sessionID string, verified bool) {
	writeJSON(w, code, map[string]any{
		"access_token":   access,
		"refresh_token":  refresh,
		"expires_at":     exp,
		"user_id":        userID,
		"session_id":     sessionID,
		"email_verified": verified,
	})
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeStatus(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	code := http.StatusInternalServerError
	switch st.Code() {
	case codes.InvalidArgument:
		code = http.StatusBadRequest
	case codes.Unauthenticated:
		code = http.StatusUnauthorized
	case codes.PermissionDenied:
		code = http.StatusForbidden
	case codes.NotFound:
		code = http.StatusNotFound
	case codes.AlreadyExists:
		code = http.StatusConflict
	case codes.FailedPrecondition:
		code = http.StatusUnauthorized
	case codes.Unavailable:
		code = http.StatusServiceUnavailable
	}
	msg := st.Message()
	if !publicMessage(msg) {
		msg = "internal"
		if code != http.StatusInternalServerError {
			msg = "invalid_argument"
		}
	}
	writeJSON(w, code, map[string]string{"error": msg})
}

func publicMessage(msg string) bool {
	switch msg {
	case "invalid_argument", "invalid_credentials", "invalid_api_key", "email_already_registered", "slug_taken", "forbidden", "not_found", "account_disabled", "refresh_reuse_detected", "unauthenticated", "invalid_reset_token", "invalid_verification_token", "invalid_google_credential", "google_identity_conflict", "google_sign_in_unavailable", "internal", "already_exists":
		return true
	default:
		return false
	}
}
