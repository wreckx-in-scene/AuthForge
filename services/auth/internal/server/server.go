package server

import (
	"context"
	"log/slog"

	"github.com/coreos/go-oidc/v3/oidc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/auth/v1"
	identityv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/identity/v1"
	sessionv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/session/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/token"
)

type Server struct {
	authv1.UnimplementedAuthServiceServer
	identity         identityv1.IdentityServiceClient
	session          sessionv1.SessionServiceClient
	signer           *token.Signer
	googleIDVerifier *oidc.IDTokenVerifier
	log              *slog.Logger
}

func New(identity identityv1.IdentityServiceClient, session sessionv1.SessionServiceClient, signer *token.Signer, googleIDVerifier *oidc.IDTokenVerifier, log *slog.Logger) *Server {
	return &Server{identity: identity, session: session, signer: signer, googleIDVerifier: googleIDVerifier, log: log}
}

func (s *Server) SignUp(ctx context.Context, req *authv1.SignUpRequest) (*authv1.SignUpResponse, error) {
	created, err := s.identity.CreateUser(ctx, &identityv1.CreateUserRequest{
		EnvironmentId: req.GetEnvironmentId(),
		ProjectId:     req.GetProjectId(),
		Email:         req.GetEmail(),
		Password:      req.GetPassword(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	return s.issue(ctx, created.GetUser())
}

func (s *Server) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	authed, err := s.identity.Authenticate(ctx, &identityv1.AuthenticateRequest{
		EnvironmentId: req.GetEnvironmentId(),
		Email:         req.GetEmail(),
		Password:      req.GetPassword(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	user := authed.GetUser()
	if user.GetProjectId() != req.GetProjectId() {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}
	issued, err := s.issue(ctx, user)
	if err != nil {
		return nil, err
	}
	return &authv1.LoginResponse{
		AccessToken:     issued.AccessToken,
		RefreshToken:    issued.RefreshToken,
		AccessExpiresAt: issued.AccessExpiresAt,
		UserId:          issued.UserId,
		SessionId:       issued.SessionId,
		EmailVerified:   issued.EmailVerified,
	}, nil
}

func (s *Server) Refresh(ctx context.Context, req *authv1.RefreshRequest) (*authv1.RefreshResponse, error) {
	rotated, err := s.session.RotateRefreshToken(ctx, &sessionv1.RotateRefreshTokenRequest{RefreshToken: req.GetRefreshToken()})
	if err != nil {
		return nil, s.pass(err)
	}
	loaded, err := s.identity.GetUser(ctx, &identityv1.GetUserRequest{
		EnvironmentId: rotated.GetEnvironmentId(),
		UserId:        rotated.GetUserId(),
	})
	if err != nil {
		s.log.Error("user lookup failed after refresh rotation", "session_id", rotated.GetSessionId(), "err", err)
		return nil, s.pass(err)
	}
	user := loaded.GetUser()
	if user.GetStatus() != "active" || user.GetProjectId() != rotated.GetProjectId() {
		_, _ = s.session.RevokeSession(ctx, &sessionv1.RevokeSessionRequest{
			SessionId:     rotated.GetSessionId(),
			EnvironmentId: rotated.GetEnvironmentId(),
		})
		return nil, status.Error(codes.PermissionDenied, "account_disabled")
	}
	access, exp, err := s.signer.SignAccess(user.GetId(), user.GetEnvironmentId(), user.GetProjectId(), rotated.GetSessionId(), user.GetEmailVerified(), now())
	if err != nil {
		s.log.Error("signing failed after refresh rotation", "session_id", rotated.GetSessionId(), "err", err)
		return nil, status.Error(codes.Internal, "internal")
	}
	return &authv1.RefreshResponse{
		AccessToken:     access,
		RefreshToken:    rotated.GetRefreshToken(),
		AccessExpiresAt: exp.Unix(),
		UserId:          user.GetId(),
		SessionId:       rotated.GetSessionId(),
		EmailVerified:   user.GetEmailVerified(),
	}, nil
}

func (s *Server) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	revoked, err := s.session.RevokeSession(ctx, &sessionv1.RevokeSessionRequest{
		RefreshToken:  req.GetRefreshToken(),
		SessionId:     req.GetSessionId(),
		EnvironmentId: req.GetEnvironmentId(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	return &authv1.LogoutResponse{SessionId: revoked.GetSessionId()}, nil
}

func (s *Server) RequestEmailVerification(ctx context.Context, req *authv1.RequestEmailVerificationRequest) (*authv1.RequestEmailVerificationResponse, error) {
	_, err := s.identity.RequestEmailVerification(ctx, &identityv1.RequestEmailVerificationRequest{
		EnvironmentId: req.GetEnvironmentId(),
		Email:         req.GetEmail(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	return &authv1.RequestEmailVerificationResponse{}, nil
}

func (s *Server) VerifyEmail(ctx context.Context, req *authv1.VerifyEmailRequest) (*authv1.VerifyEmailResponse, error) {
	verified, err := s.identity.VerifyEmail(ctx, &identityv1.VerifyEmailRequest{
		EnvironmentId: req.GetEnvironmentId(),
		Token:         req.GetToken(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	return &authv1.VerifyEmailResponse{Verified: verified.GetVerified()}, nil
}

func (s *Server) CreateGoogleNonce(ctx context.Context, req *authv1.CreateGoogleNonceRequest) (*authv1.CreateGoogleNonceResponse, error) {
	if s.googleIDVerifier == nil {
		return nil, status.Error(codes.Unavailable, "google_sign_in_unavailable")
	}
	created, err := s.identity.CreateGoogleNonce(ctx, &identityv1.CreateGoogleNonceRequest{
		EnvironmentId: req.GetEnvironmentId(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	return &authv1.CreateGoogleNonceResponse{Nonce: created.GetNonce()}, nil
}

func (s *Server) GoogleLogin(ctx context.Context, req *authv1.GoogleLoginRequest) (*authv1.GoogleLoginResponse, error) {
	if s.googleIDVerifier == nil {
		return nil, status.Error(codes.Unavailable, "google_sign_in_unavailable")
	}
	claims, err := verifyGoogleCredential(ctx, s.googleIDVerifier, req.GetIdToken(), req.GetNonce())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid_google_credential")
	}
	authed, err := s.identity.AuthenticateGoogle(ctx, &identityv1.AuthenticateGoogleRequest{
		EnvironmentId: req.GetEnvironmentId(),
		ProjectId:     req.GetProjectId(),
		Subject:       claims.Subject,
		Email:         claims.Email,
		Nonce:         req.GetNonce(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	user := authed.GetUser()
	if user.GetProjectId() != req.GetProjectId() {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}
	issued, err := s.issue(ctx, user)
	if err != nil {
		return nil, err
	}
	return &authv1.GoogleLoginResponse{
		AccessToken:     issued.GetAccessToken(),
		RefreshToken:    issued.GetRefreshToken(),
		AccessExpiresAt: issued.GetAccessExpiresAt(),
		UserId:          issued.GetUserId(),
		SessionId:       issued.GetSessionId(),
		EmailVerified:   issued.GetEmailVerified(),
	}, nil
}

func (s *Server) Authorize(ctx context.Context, req *authv1.AuthorizeRequest) (*authv1.AuthorizeResponse, error) {
	var user *identityv1.User
	if req.GetSignup() {
		created, err := s.identity.CreateUser(ctx, &identityv1.CreateUserRequest{
			EnvironmentId: req.GetEnvironmentId(),
			ProjectId:     req.GetProjectId(),
			Email:         req.GetEmail(),
			Password:      req.GetPassword(),
		})
		if err != nil {
			return nil, s.pass(err)
		}
		user = created.GetUser()
	} else {
		authed, err := s.identity.Authenticate(ctx, &identityv1.AuthenticateRequest{
			EnvironmentId: req.GetEnvironmentId(),
			Email:         req.GetEmail(),
			Password:      req.GetPassword(),
		})
		if err != nil {
			return nil, s.pass(err)
		}
		user = authed.GetUser()
	}
	if user.GetEnvironmentId() != req.GetEnvironmentId() || user.GetProjectId() != req.GetProjectId() {
		return nil, status.Error(codes.PermissionDenied, "forbidden")
	}
	if user.GetStatus() != "active" {
		return nil, status.Error(codes.PermissionDenied, "account_disabled")
	}
	issued, err := s.session.IssueAuthorizationCode(ctx, &sessionv1.IssueAuthorizationCodeRequest{
		EnvironmentId: req.GetEnvironmentId(), ProjectId: req.GetProjectId(),
		UserId: user.GetId(), ClientId: req.GetClientId(), RedirectUri: req.GetRedirectUri(),
		CodeChallenge: req.GetCodeChallenge(), Nonce: req.GetNonce(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	return &authv1.AuthorizeResponse{Code: issued.GetCode(), ExpiresAt: issued.GetExpiresAt()}, nil
}

func (s *Server) ExchangeAuthorizationCode(ctx context.Context, req *authv1.ExchangeAuthorizationCodeRequest) (*authv1.ExchangeAuthorizationCodeResponse, error) {
	issued, err := s.session.ExchangeAuthorizationCode(ctx, &sessionv1.ExchangeAuthorizationCodeRequest{
		Code: req.GetCode(), ClientId: req.GetClientId(),
		RedirectUri: req.GetRedirectUri(), CodeVerifier: req.GetCodeVerifier(),
	})
	if err != nil {
		return nil, s.pass(err)
	}
	loaded, err := s.identity.GetUser(ctx, &identityv1.GetUserRequest{
		EnvironmentId: issued.GetEnvironmentId(), UserId: issued.GetUserId(),
	})
	if err != nil {
		s.revokeFailedAuthorizationExchange(ctx, issued.GetSessionId(), issued.GetEnvironmentId(), err)
		return nil, s.pass(err)
	}
	user := loaded.GetUser()
	if user.GetStatus() != "active" || user.GetEnvironmentId() != issued.GetEnvironmentId() ||
		user.GetProjectId() != issued.GetProjectId() {
		s.revokeFailedAuthorizationExchange(ctx, issued.GetSessionId(), issued.GetEnvironmentId(), nil)
		return nil, status.Error(codes.PermissionDenied, "account_disabled")
	}
	now := now()
	access, accessExpires, err := s.signer.SignAccess(
		user.GetId(), user.GetEnvironmentId(), user.GetProjectId(), issued.GetSessionId(),
		user.GetEmailVerified(), now,
	)
	if err != nil {
		s.revokeFailedAuthorizationExchange(ctx, issued.GetSessionId(), issued.GetEnvironmentId(), err)
		return nil, status.Error(codes.Internal, "internal")
	}
	idToken, _, err := s.signer.SignIDToken(
		user.GetId(), req.GetClientId(), user.GetEmail(), issued.GetNonce(),
		user.GetEmailVerified(), now,
	)
	if err != nil {
		s.revokeFailedAuthorizationExchange(ctx, issued.GetSessionId(), issued.GetEnvironmentId(), err)
		return nil, status.Error(codes.Internal, "internal")
	}
	return &authv1.ExchangeAuthorizationCodeResponse{
		AccessToken: access, RefreshToken: issued.GetRefreshToken(), IdToken: idToken,
		AccessExpiresAt: accessExpires.Unix(), UserId: user.GetId(),
		SessionId: issued.GetSessionId(), EmailVerified: user.GetEmailVerified(),
	}, nil
}

func (s *Server) revokeFailedAuthorizationExchange(ctx context.Context, sessionID, environmentID string, cause error) {
	_, revokeErr := s.session.RevokeSession(ctx, &sessionv1.RevokeSessionRequest{
		SessionId: sessionID, EnvironmentId: environmentID,
	})
	s.log.Error("authorization code exchange failed after session creation",
		"session_id", sessionID, "cause", cause, "revoke_err", revokeErr)
}

func (s *Server) VerificationKeys(context.Context, *authv1.VerificationKeysRequest) (*authv1.VerificationKeysResponse, error) {
	return &authv1.VerificationKeysResponse{
		Kid:       s.signer.KID(),
		PublicKey: s.signer.PublicKey(),
		Alg:       "EdDSA",
	}, nil
}

func (s *Server) issue(ctx context.Context, user *identityv1.User) (*authv1.SignUpResponse, error) {
	created, err := s.session.CreateSession(ctx, &sessionv1.CreateSessionRequest{
		EnvironmentId: user.GetEnvironmentId(),
		ProjectId:     user.GetProjectId(),
		UserId:        user.GetId(),
	})
	if err != nil {
		s.log.Error("session create failed after identity commit", "user_id", user.GetId(), "err", err)
		return nil, s.pass(err)
	}
	access, exp, err := s.signer.SignAccess(user.GetId(), user.GetEnvironmentId(), user.GetProjectId(), created.GetSessionId(), user.GetEmailVerified(), now())
	if err != nil {
		_, revokeErr := s.session.RevokeSession(ctx, &sessionv1.RevokeSessionRequest{
			SessionId:     created.GetSessionId(),
			EnvironmentId: created.GetEnvironmentId(),
		})
		s.log.Error("signing failed", "session_id", created.GetSessionId(), "revoke_err", revokeErr, "err", err)
		return nil, status.Error(codes.Internal, "internal")
	}
	return &authv1.SignUpResponse{
		AccessToken:     access,
		RefreshToken:    created.GetRefreshToken(),
		AccessExpiresAt: exp.Unix(),
		UserId:          user.GetId(),
		SessionId:       created.GetSessionId(),
		EmailVerified:   user.GetEmailVerified(),
	}, nil
}

func (s *Server) pass(err error) error {
	if st, ok := status.FromError(err); ok && st.Code() != codes.OK {
		return err
	}
	s.log.Error("downstream call failed", "err", err)
	return status.Error(codes.Internal, "internal")
}
