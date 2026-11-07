package server

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tenantv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/tenant/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/apikey"
	"github.com/wreckx-in-scene/AuthForge/internal/digest"
	"github.com/wreckx-in-scene/AuthForge/internal/errs"
	"github.com/wreckx-in-scene/AuthForge/internal/mailer"
	"github.com/wreckx-in-scene/AuthForge/internal/normalize"
	"github.com/wreckx-in-scene/AuthForge/internal/password"
	"github.com/wreckx-in-scene/AuthForge/internal/statusx"
	"github.com/wreckx-in-scene/AuthForge/internal/token"
	"github.com/wreckx-in-scene/AuthForge/services/tenant/internal/store"
)

type Server struct {
	tenantv1.UnimplementedTenantServiceServer
	store       *store.Store
	secret      []byte
	issuer      string
	consoleTTL  time.Duration
	resetURL    string
	resetSender mailer.PasswordResetSender
	log         *slog.Logger
}

func New(st *store.Store, secret []byte, issuer string, consoleTTL time.Duration, resetURL string, resetMailer mailer.PasswordResetSender, log *slog.Logger) *Server {
	return &Server{
		store: st, secret: secret, issuer: issuer, consoleTTL: consoleTTL,
		resetURL: resetURL, resetSender: resetMailer, log: log,
	}
}

func (s *Server) CreateAccount(ctx context.Context, req *tenantv1.CreateAccountRequest) (*tenantv1.CreateAccountResponse, error) {
	display, normalized, err := normalize.Email(req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	name, err := personName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	hash, err := password.Hash(req.GetPassword())
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	account, err := s.store.CreateAccount(ctx, display, normalized, name, hash)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "email_already_registered")
	}
	raw, exp, err := s.consoleToken(account.ID)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return &tenantv1.CreateAccountResponse{
		AccessToken: raw,
		ExpiresAt:   exp.Unix(),
		Account:     toAccount(account),
	}, nil
}

func (s *Server) LoginAccount(ctx context.Context, req *tenantv1.LoginAccountRequest) (*tenantv1.LoginAccountResponse, error) {
	_, normalized, err := normalize.Email(req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid_credentials")
	}
	account, hash, err := s.store.FindAccount(ctx, normalized)
	if errors.Is(err, errs.ErrNotFound) {
		_, _ = password.Verify(req.GetPassword(), password.Dummy)
		return nil, status.Error(codes.Unauthenticated, "invalid_credentials")
	}
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	ok, err := password.Verify(req.GetPassword(), hash)
	if err != nil || !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid_credentials")
	}
	raw, exp, err := s.consoleToken(account.ID)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return &tenantv1.LoginAccountResponse{
		AccessToken: raw,
		ExpiresAt:   exp.Unix(),
		Account:     toAccount(account),
	}, nil
}

func (s *Server) RequestPasswordReset(ctx context.Context, req *tenantv1.RequestPasswordResetRequest) (*tenantv1.RequestPasswordResetResponse, error) {
	_, normalized, err := normalize.Email(req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	email, resetToken, _, err := s.store.RequestPasswordReset(ctx, normalized)
	if errors.Is(err, errs.ErrNotFound) {
		return &tenantv1.RequestPasswordResetResponse{}, nil
	}
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	resetLink, err := mailer.PasswordResetURL(s.resetURL, resetToken)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	if err := s.resetSender.SendPasswordReset(ctx, email, resetLink); err != nil {
		s.log.Error("password reset email delivery failed", "err", err)
	}
	return &tenantv1.RequestPasswordResetResponse{}, nil
}

func (s *Server) ResetPassword(ctx context.Context, req *tenantv1.ResetPasswordRequest) (*tenantv1.ResetPasswordResponse, error) {
	if req.GetResetToken() == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid_reset_token")
	}
	hash, err := password.Hash(req.GetPassword())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	_, err = s.store.ResetPassword(ctx, req.GetResetToken(), hash)
	if errors.Is(err, errs.ErrUnauthenticated) {
		return nil, status.Error(codes.Unauthenticated, "invalid_reset_token")
	}
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return &tenantv1.ResetPasswordResponse{Ok: true}, nil
}

func (s *Server) CreateOrganization(ctx context.Context, req *tenantv1.CreateOrganizationRequest) (*tenantv1.CreateOrganizationResponse, error) {
	name, err := personName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	slug, err := normalize.Slug(req.GetSlug(), name)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	org, err := s.store.CreateOrganization(ctx, req.GetAccountId(), name, slug)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "slug_taken")
	}
	return &tenantv1.CreateOrganizationResponse{Id: org.ID, Name: org.Name, Slug: org.Slug}, nil
}

func (s *Server) CreateProject(ctx context.Context, req *tenantv1.CreateProjectRequest) (*tenantv1.CreateProjectResponse, error) {
	name, err := personName(req.GetName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	slug, err := normalize.Slug(req.GetSlug(), name)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	project, err := s.store.CreateProject(ctx, req.GetAccountId(), req.GetOrganizationId(), name, slug)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "slug_taken")
	}
	return &tenantv1.CreateProjectResponse{
		Id:             project.ID,
		OrganizationId: project.OrganizationID,
		Name:           project.Name,
		Slug:           project.Slug,
	}, nil
}

func (s *Server) CreateEnvironment(ctx context.Context, req *tenantv1.CreateEnvironmentRequest) (*tenantv1.CreateEnvironmentResponse, error) {
	name, err := normalize.Slug(req.GetName(), "")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	env, err := s.store.CreateEnvironment(ctx, req.GetAccountId(), req.GetProjectId(), name)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "already_exists")
	}
	return &tenantv1.CreateEnvironmentResponse{
		Id:             env.ID,
		ProjectId:      env.ProjectID,
		OrganizationId: env.OrganizationID,
		Name:           env.Name,
	}, nil
}

func (s *Server) IssueAPIKey(ctx context.Context, req *tenantv1.IssueAPIKeyRequest) (*tenantv1.IssueAPIKeyResponse, error) {
	class := req.GetKeyClass()
	if class != "publishable" && class != "secret" {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	envName := ""
	// The prefix depends on the environment name, so resolve membership and name together in the store.
	// Generate after we know the name. A two-step call keeps key material out of the membership query.
	resolved, err := s.store.ResolveEnvironmentName(ctx, req.GetAccountId(), req.GetEnvironmentId())
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	envName = resolved
	plain, prefix, sum, err := apikey.Generate(envName, class)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	issued, err := s.store.InsertAPIKey(ctx, req.GetAccountId(), req.GetEnvironmentId(), class, prefix, sum)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return &tenantv1.IssueAPIKeyResponse{
		Id:            issued.ID,
		EnvironmentId: issued.EnvironmentID,
		Key:           plain,
		KeyClass:      issued.KeyClass,
		Prefix:        issued.Prefix,
	}, nil
}

func (s *Server) ResolveAPIKey(ctx context.Context, req *tenantv1.ResolveAPIKeyRequest) (*tenantv1.ResolveAPIKeyResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	resolved, err := s.store.ResolveAPIKey(ctx, digest.SHA256(req.GetKey()))
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return &tenantv1.ResolveAPIKeyResponse{
		KeyId:           resolved.KeyID,
		OrganizationId:  resolved.OrganizationID,
		ProjectId:       resolved.ProjectID,
		EnvironmentId:   resolved.EnvironmentID,
		KeyClass:        resolved.KeyClass,
		EnvironmentName: resolved.EnvironmentName,
	}, nil
}

func (s *Server) consoleToken(accountID string) (string, time.Time, error) {
	return token.SignConsole(s.secret, s.issuer, accountID, s.consoleTTL, time.Now().UTC())
}

func toAccount(account store.Account) *tenantv1.Account {
	return &tenantv1.Account{Id: account.ID, Email: account.Email, Name: account.Name}
}

func personName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 80 {
		return "", errs.ErrInvalid
	}
	return name, nil
}
