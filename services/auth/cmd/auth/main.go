package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	authv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/auth/v1"
	identityv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/identity/v1"
	sessionv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/session/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/config"
	"github.com/wreckx-in-scene/AuthForge/internal/health"
	"github.com/wreckx-in-scene/AuthForge/internal/serve"
	"github.com/wreckx-in-scene/AuthForge/internal/token"
	"github.com/wreckx-in-scene/AuthForge/services/auth/internal/server"
)

func main() {
	health.ProbeOrContinue()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, log); err != nil {
		log.Error("stopped", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *slog.Logger) error {
	identityAddr, err := config.Require("IDENTITY_GRPC_ADDR")
	if err != nil {
		return err
	}
	sessionAddr, err := config.Require("SESSION_GRPC_ADDR")
	if err != nil {
		return err
	}
	issuer, err := config.Require("ISSUER")
	if err != nil {
		return err
	}
	seedRaw, err := config.Require("SIGNING_SEED")
	if err != nil {
		return err
	}
	accessTTL, err := config.Duration("ACCESS_TTL", "10m")
	if err != nil {
		return err
	}
	seed, err := token.ParseSeed(seedRaw)
	if err != nil {
		return err
	}
	signer, err := token.NewSigner(seed, issuer, accessTTL)
	if err != nil {
		return err
	}
	identityConn, err := grpc.NewClient(identityAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer identityConn.Close()
	sessionConn, err := grpc.NewClient(sessionAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer sessionConn.Close()

	api := server.New(identityv1.NewIdentityServiceClient(identityConn), sessionv1.NewSessionServiceClient(sessionConn), signer, log)
	return serve.GRPC(ctx, ":"+config.String("HEALTH_PORT", "8081"), ":"+config.String("GRPC_PORT", "50051"), func(gs *grpc.Server) {
		authv1.RegisterAuthServiceServer(gs, api)
	})
}
