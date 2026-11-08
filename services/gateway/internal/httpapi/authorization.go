package httpapi

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/auth/v1"
	tenantv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/tenant/v1"
	"github.com/wreckx-in-scene/AuthForge/services/gateway/internal/dashboard"
)

type authorizationRequest struct {
	client        *tenantv1.OAuthClient
	redirectURI   string
	state         string
	codeChallenge string
	nonce         string
}

func (s *Server) openIDConfiguration(w http.ResponseWriter, _ *http.Request) {
	issuer := s.issuer
	endpointBase := strings.TrimRight(issuer, "/")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                endpointBase + "/authorize",
		"token_endpoint":                        endpointBase + "/v1/token",
		"jwks_uri":                              endpointBase + "/.well-known/jwks.json",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"EdDSA"},
		"scopes_supported":                      []string{"openid", "email"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func (s *Server) openIDKeys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	keys, err := s.auth.VerificationKeys(ctx, &authv1.VerificationKeysRequest{})
	if err != nil {
		writeStatus(w, err)
		return
	}
	if keys.GetAlg() != "EdDSA" || len(keys.GetPublicKey()) != ed25519.PublicKeySize {
		s.log.Error("auth service returned an invalid OIDC verification key")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"keys": []map[string]string{{
			"kty": "OKP", "crv": "Ed25519", "use": "sig", "alg": "EdDSA",
			"kid": keys.GetKid(), "x": base64.RawURLEncoding.EncodeToString(keys.GetPublicKey()),
		}},
	})
}

func (s *Server) authorizePage(w http.ResponseWriter, r *http.Request) {
	_, err := s.resolveAuthorizationRequest(r.Context(), r.URL.Query())
	if err != nil {
		writeAuthorizationPageError(w, err)
		return
	}
	dashboard.Handler().ServeHTTP(w, r)
}

func (s *Server) authorizeUser(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if contentType := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]); contentType != "application/x-www-form-urlencoded" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	for key := range r.PostForm {
		switch key {
		case "client_id", "redirect_uri", "response_type", "scope", "state",
			"code_challenge", "code_challenge_method", "nonce",
			"action", "email", "password":
		default:
			writeOAuthError(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	if len(r.PostForm["action"]) != 1 || len(r.PostForm["email"]) != 1 || len(r.PostForm["password"]) != 1 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	action := r.PostForm.Get("action")
	if action != "login" && action != "signup" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	email := r.PostForm.Get("email")
	password := r.PostForm.Get("password")
	if len(email) > 320 || len(password) > 128 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	authorizationValues := make(url.Values)
	for _, key := range []string{
		"client_id", "redirect_uri", "response_type", "scope", "state",
		"code_challenge", "code_challenge_method", "nonce",
	} {
		if values, ok := r.PostForm[key]; ok {
			authorizationValues[key] = values
		}
	}
	request, err := s.resolveAuthorizationRequest(r.Context(), authorizationValues)
	if err != nil {
		if isRequestValidationError(err) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		} else {
			writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	authorized, err := s.auth.Authorize(ctx, &authv1.AuthorizeRequest{
		EnvironmentId: request.client.GetEnvironmentId(),
		ProjectId:     request.client.GetProjectId(),
		ClientId:      request.client.GetClientId(),
		RedirectUri:   request.redirectURI,
		CodeChallenge: request.codeChallenge,
		Nonce:         request.nonce,
		Email:         email,
		Password:      password,
		Signup:        action == "signup",
	})
	if err != nil {
		writeStatus(w, err)
		return
	}
	callback, err := url.Parse(request.redirectURI)
	if err != nil {
		s.log.Error("registered OAuth callback could not be parsed", "client_id", request.client.GetClientId(), "err", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	query := callback.Query()
	query.Set("code", authorized.GetCode())
	query.Set("state", request.state)
	callback.RawQuery = query.Encode()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		writeJSON(w, http.StatusOK, map[string]string{"redirect_url": callback.String()})
		return
	}
	http.Redirect(w, r, callback.String(), http.StatusSeeOther)
}

func (s *Server) tokenEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if contentType := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]); contentType != "application/x-www-form-urlencoded" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if len(r.PostForm["grant_type"]) != 1 ||
		len(r.PostForm["client_id"]) != 1 ||
		len(r.PostForm["code"]) != 1 ||
		len(r.PostForm["redirect_uri"]) != 1 ||
		len(r.PostForm["code_verifier"]) != 1 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	for key := range r.PostForm {
		switch key {
		case "grant_type", "client_id", "code", "redirect_uri", "code_verifier":
		default:
			writeOAuthError(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	clientID := r.PostForm.Get("client_id")
	redirectURI := r.PostForm.Get("redirect_uri")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	clientResponse, err := s.tenant.ResolveOAuthClient(ctx, &tenantv1.ResolveOAuthClientRequest{ClientId: clientID})
	if err != nil {
		cancel()
		if isInvalidClientError(err) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client")
		} else {
			writeOAuthError(w, http.StatusServiceUnavailable, "temporarily_unavailable")
		}
		return
	}
	if clientResponse.GetClient() == nil || !registeredRedirect(clientResponse.GetClient(), redirectURI) {
		cancel()
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		if !registeredOrigin(clientResponse.GetClient(), origin) {
			cancel()
			writeOAuthError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
	}
	exchanged, err := s.auth.ExchangeAuthorizationCode(ctx, &authv1.ExchangeAuthorizationCodeRequest{
		Code: r.PostForm.Get("code"), ClientId: clientID,
		RedirectUri: redirectURI, CodeVerifier: r.PostForm.Get("code_verifier"),
	})
	cancel()
	if err != nil {
		if isOAuthGrantError(err) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant")
			return
		}
		writeOAuthError(w, http.StatusInternalServerError, "server_error")
		return
	}
	expiresIn := exchanged.GetAccessExpiresAt() - time.Now().Unix()
	if expiresIn < 0 {
		expiresIn = 0
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  exchanged.GetAccessToken(),
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"refresh_token": exchanged.GetRefreshToken(),
		"id_token":      exchanged.GetIdToken(),
		"scope":         "openid email",
	})
}

func (s *Server) resolveAuthorizationRequest(ctx context.Context, values url.Values) (authorizationRequest, error) {
	allowed := map[string]bool{
		"client_id": true, "redirect_uri": true, "response_type": true, "scope": true,
		"state": true, "code_challenge": true, "code_challenge_method": true, "nonce": true,
	}
	for key := range values {
		if !allowed[key] {
			return authorizationRequest{}, errors.New("unsupported authorization parameter")
		}
	}
	clientID, err := requiredValue(values, "client_id", 64)
	if err != nil {
		return authorizationRequest{}, err
	}
	redirectURI, err := requiredValue(values, "redirect_uri", 2048)
	if err != nil {
		return authorizationRequest{}, err
	}
	responseType, err := requiredValue(values, "response_type", 16)
	if err != nil || responseType != "code" {
		return authorizationRequest{}, errors.New("unsupported response type")
	}
	scope, err := requiredValue(values, "scope", 128)
	if err != nil || !supportedOIDCScope(scope) {
		return authorizationRequest{}, errors.New("unsupported scope")
	}
	state, err := requiredValue(values, "state", 512)
	if err != nil || strings.ContainsAny(state, "\x00\r\n") {
		return authorizationRequest{}, errors.New("invalid state")
	}
	method, err := requiredValue(values, "code_challenge_method", 8)
	if err != nil || method != "S256" {
		return authorizationRequest{}, errors.New("unsupported PKCE method")
	}
	challenge, err := requiredValue(values, "code_challenge", 64)
	if err != nil || !validS256Challenge(challenge) {
		return authorizationRequest{}, errors.New("invalid PKCE challenge")
	}
	nonce, err := optionalValue(values, "nonce", 512)
	if err != nil || strings.ContainsAny(nonce, "\x00\r\n") {
		return authorizationRequest{}, errors.New("invalid nonce")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resolved, err := s.tenant.ResolveOAuthClient(ctx, &tenantv1.ResolveOAuthClientRequest{ClientId: clientID})
	if err != nil {
		return authorizationRequest{}, err
	}
	client := resolved.GetClient()
	if client == nil || !registeredRedirect(client, redirectURI) {
		return authorizationRequest{}, errors.New("unknown OAuth client or redirect URI")
	}
	return authorizationRequest{
		client: client, redirectURI: redirectURI, state: state,
		codeChallenge: challenge, nonce: nonce,
	}, nil
}

func requiredValue(values url.Values, key string, maxLength int) (string, error) {
	items := values[key]
	if len(items) != 1 || items[0] == "" || len(items[0]) > maxLength {
		return "", errors.New("invalid request parameter")
	}
	return items[0], nil
}

func optionalValue(values url.Values, key string, maxLength int) (string, error) {
	items := values[key]
	if len(items) > 1 || (len(items) == 1 && (items[0] == "" || len(items[0]) > maxLength)) {
		return "", errors.New("invalid request parameter")
	}
	if len(items) == 0 {
		return "", nil
	}
	return items[0], nil
}

func supportedOIDCScope(scope string) bool {
	scopes := strings.Fields(scope)
	return len(scopes) == 2 &&
		((scopes[0] == "openid" && scopes[1] == "email") ||
			(scopes[0] == "email" && scopes[1] == "openid"))
}

func validS256Challenge(challenge string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(challenge)
	return err == nil && len(raw) == sha256.Size && base64.RawURLEncoding.EncodeToString(raw) == challenge
}

func registeredRedirect(client *tenantv1.OAuthClient, redirectURI string) bool {
	for _, registered := range client.GetRedirectUris() {
		if redirectURI == registered {
			return true
		}
	}
	return false
}

func registeredOrigin(client *tenantv1.OAuthClient, origin string) bool {
	requestURL, err := url.Parse(origin)
	if err != nil || requestURL.Path != "" || requestURL.RawQuery != "" ||
		requestURL.Fragment != "" || requestURL.User != nil {
		return false
	}
	requestOrigin, ok := normalizedOrigin(origin)
	if !ok {
		return false
	}
	for _, redirectURI := range client.GetRedirectUris() {
		registeredOrigin, ok := normalizedOrigin(redirectURI)
		if ok && requestOrigin == registeredOrigin {
			return true
		}
	}
	return false
}

func normalizedOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.Opaque != "" || parsed.Fragment != "" {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host = net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
	return scheme + "://" + host, true
}

func isOAuthGrantError(err error) bool {
	code := status.Code(err)
	return code == codes.InvalidArgument || code == codes.Unauthenticated ||
		code == codes.PermissionDenied || code == codes.NotFound
}

func isRequestValidationError(err error) bool {
	if _, isStatus := status.FromError(err); !isStatus {
		return true
	}
	code := status.Code(err)
	return code == codes.InvalidArgument || code == codes.NotFound || code == codes.PermissionDenied
}

func isInvalidClientError(err error) bool {
	code := status.Code(err)
	return code == codes.InvalidArgument || code == codes.NotFound
}

func writeAuthorizationPageError(w http.ResponseWriter, err error) {
	statusCode := http.StatusBadRequest
	if _, isStatus := status.FromError(err); isStatus && !isInvalidClientError(err) {
		statusCode = http.StatusServiceUnavailable
	}
	http.Error(w, "invalid_request", statusCode)
}

func writeOAuthError(w http.ResponseWriter, statusCode int, code string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, statusCode, map[string]string{"error": code})
}
