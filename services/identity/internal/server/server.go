package server

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/identity/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/errs"
	"github.com/wreckx-in-scene/AuthForge/internal/id"
	"github.com/wreckx-in-scene/AuthForge/internal/mailer"
	"github.com/wreckx-in-scene/AuthForge/internal/normalize"
	"github.com/wreckx-in-scene/AuthForge/internal/password"
	"github.com/wreckx-in-scene/AuthForge/internal/statusx"
	"github.com/wreckx-in-scene/AuthForge/services/identity/internal/store"
)

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	store           *store.Store
	verificationURL string
	mailer          mailer.EmailVerificationSender
	log             *slog.Logger
}

func New(st *store.Store, verificationURL string, emailSender mailer.EmailVerificationSender, log *slog.Logger) *Server {
	return &Server{store: st, verificationURL: verificationURL, mailer: emailSender, log: log}
}

func (s *Server) CreateUser(ctx context.Context, req *identityv1.CreateUserRequest) (*identityv1.CreateUserResponse, error) {
	if err := id.Parse(req.GetEnvironmentId(), req.GetProjectId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	display, normalized, err := normalize.Email(req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	hash, err := password.Hash(req.GetPassword())
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	user, err := s.store.CreateUser(ctx, req.GetEnvironmentId(), req.GetProjectId(), display, normalized, hash)
	if err != nil {
		return nil, statusx.Fail(s.log, err, "email_already_registered")
	}
	if err := s.sendVerification(ctx, user.EnvironmentID, user.Email, normalized); err != nil {
		s.log.Error("verification email could not be sent after signup", "user_id", user.ID, "err", err)
	}
	return &identityv1.CreateUserResponse{User: toProto(user)}, nil
}

func (s *Server) Authenticate(ctx context.Context, req *identityv1.AuthenticateRequest) (*identityv1.AuthenticateResponse, error) {
	if err := id.Parse(req.GetEnvironmentId()); err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid_credentials")
	}
	_, normalized, err := normalize.Email(req.GetEmail())
	if err != nil {
		_, _ = password.Verify(req.GetPassword(), password.Dummy)
		return nil, status.Error(codes.Unauthenticated, "invalid_credentials")
	}
	user, hash, err := s.store.FindByEmail(ctx, req.GetEnvironmentId(), normalized)
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
	if user.Status != "active" {
		return nil, status.Error(codes.PermissionDenied, "account_disabled")
	}
	return &identityv1.AuthenticateResponse{User: toProto(user)}, nil
}

func (s *Server) GetUser(ctx context.Context, req *identityv1.GetUserRequest) (*identityv1.GetUserResponse, error) {
	user, err := s.store.GetUser(ctx, req.GetEnvironmentId(), req.GetUserId())
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return &identityv1.GetUserResponse{User: toProto(user)}, nil
}

func (s *Server) RequestEmailVerification(ctx context.Context, req *identityv1.RequestEmailVerificationRequest) (*identityv1.RequestEmailVerificationResponse, error) {
	_, normalized, err := normalize.Email(req.GetEmail())
	if err != nil || id.Parse(req.GetEnvironmentId()) != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid_argument")
	}
	email, verificationToken, err := s.store.CreateEmailVerificationToken(ctx, req.GetEnvironmentId(), normalized)
	if errors.Is(err, errs.ErrNotFound) {
		return &identityv1.RequestEmailVerificationResponse{}, nil
	}
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	if verificationToken != "" {
		if err := s.sendVerificationLink(ctx, req.GetEnvironmentId(), email, verificationToken); err != nil {
			s.log.Error("verification email delivery failed", "err", err)
		}
	}
	return &identityv1.RequestEmailVerificationResponse{}, nil
}

func (s *Server) VerifyEmail(ctx context.Context, req *identityv1.VerifyEmailRequest) (*identityv1.VerifyEmailResponse, error) {
	if req.GetToken() == "" || id.Parse(req.GetEnvironmentId()) != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid_verification_token")
	}
	if err := s.store.VerifyEmail(ctx, req.GetEnvironmentId(), req.GetToken()); err != nil {
		if errors.Is(err, errs.ErrUnauthenticated) || errors.Is(err, errs.ErrNotFound) {
			return nil, status.Error(codes.Unauthenticated, "invalid_verification_token")
		}
		return nil, statusx.Fail(s.log, err, "")
	}
	return &identityv1.VerifyEmailResponse{Verified: true}, nil
}

func (s *Server) sendVerification(ctx context.Context, environmentID, email, normalizedEmail string) error {
	_, verificationToken, err := s.store.CreateEmailVerificationToken(ctx, environmentID, normalizedEmail)
	if err != nil {
		return err
	}
	if verificationToken == "" {
		return nil
	}
	return s.sendVerificationLink(ctx, environmentID, email, verificationToken)
}

func (s *Server) sendVerificationLink(ctx context.Context, environmentID, email, verificationToken string) error {
	link, err := mailer.EmailVerificationURL(s.verificationURL, environmentID, verificationToken)
	if err != nil {
		return err
	}
	return s.mailer.SendEmailVerification(ctx, email, link)
}

func toProto(user store.User) *identityv1.User {
	return &identityv1.User{
		Id:            user.ID,
		EnvironmentId: user.EnvironmentID,
		ProjectId:     user.ProjectID,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		Status:        user.Status,
	}
}
