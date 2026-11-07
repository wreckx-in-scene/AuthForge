package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	tenantv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/tenant/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/config"
	"github.com/wreckx-in-scene/AuthForge/internal/database"
	"github.com/wreckx-in-scene/AuthForge/internal/health"
	"github.com/wreckx-in-scene/AuthForge/internal/serve"
	"github.com/wreckx-in-scene/AuthForge/services/tenant/internal/server"
	"github.com/wreckx-in-scene/AuthForge/services/tenant/internal/store"
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
	databaseURL, err := config.Require("DATABASE_URL")
	if err != nil {
		return err
	}
	secret, err := config.Require("CONSOLE_TOKEN_SECRET")
	if err != nil {
		return err
	}
	issuer, err := config.Require("ISSUER")
	if err != nil {
		return err
	}
	consoleTTL, err := config.Duration("CONSOLE_TTL", "12h")
	if err != nil {
		return err
	}
	if len(secret) < 32 {
		return errString("CONSOLE_TOKEN_SECRET must be at least 32 bytes")
	}
	pool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	api := server.New(store.New(pool), []byte(secret), issuer, consoleTTL, log)
	return serve.GRPC(ctx, ":"+config.String("HEALTH_PORT", "8081"), ":"+config.String("GRPC_PORT", "50051"), func(gs *grpc.Server) {
		tenantv1.RegisterTenantServiceServer(gs, api)
	})
}

type errString string

func (e errString) Error() string { return string(e) }
