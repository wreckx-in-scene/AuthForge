package server

import (
	"context"
	"log/slog"

	sessionv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/session/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/statusx"
	"github.com/wreckx-in-scene/AuthForge/services/session/internal/store"
)

type Server struct {
	sessionv1.UnimplementedSessionServiceServer
	store *store.Store
	log   *slog.Logger
}

func New(st *store.Store, log *slog.Logger) *Server {
	return &Server{store: st, log: log}
}

func (s *Server) CreateSession(ctx context.Context, req *sessionv1.CreateSessionRequest) (*sessionv1.CreateSessionResponse, error) {
	issued, err := s.store.Create(ctx, req.GetEnvironmentId(), req.GetProjectId(), req.GetUserId())
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return toProto(issued), nil
}

func (s *Server) RotateRefreshToken(ctx context.Context, req *sessionv1.RotateRefreshTokenRequest) (*sessionv1.RotateRefreshTokenResponse, error) {
	issued, err := s.store.Rotate(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return &sessionv1.RotateRefreshTokenResponse{
		SessionId:        issued.SessionID,
		EnvironmentId:    issued.EnvironmentID,
		ProjectId:        issued.ProjectID,
		UserId:           issued.UserID,
		RefreshToken:     issued.RefreshToken,
		RefreshExpiresAt: issued.ExpiresAt.Unix(),
	}, nil
}

func (s *Server) RevokeSession(ctx context.Context, req *sessionv1.RevokeSessionRequest) (*sessionv1.RevokeSessionResponse, error) {
	sessionID, err := s.store.Revoke(ctx, req.GetRefreshToken(), req.GetSessionId(), req.GetEnvironmentId())
	if err != nil {
		return nil, statusx.Fail(s.log, err, "")
	}
	return &sessionv1.RevokeSessionResponse{SessionId: sessionID}, nil
}

func toProto(issued store.Issued) *sessionv1.CreateSessionResponse {
	return &sessionv1.CreateSessionResponse{
		SessionId:        issued.SessionID,
		EnvironmentId:    issued.EnvironmentID,
		ProjectId:        issued.ProjectID,
		UserId:           issued.UserID,
		RefreshToken:     issued.RefreshToken,
		RefreshExpiresAt: issued.ExpiresAt.Unix(),
	}
}
