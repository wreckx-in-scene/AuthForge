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
	"github.com/wreckx-in-scene/AuthForge/internal/normalize"
	"github.com/wreckx-in-scene/AuthForge/internal/password"
	"github.com/wreckx-in-scene/AuthForge/internal/statusx"
	"github.com/wreckx-in-scene/AuthForge/services/identity/internal/store"
)

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	store *store.Store
	log   *slog.Logger
}

func New(st *store.Store, log *slog.Logger) *Server {
	return &Server{store: st, log: log}
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
