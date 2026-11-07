package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	sessionv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/session/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/config"
	"github.com/wreckx-in-scene/AuthForge/internal/database"
	"github.com/wreckx-in-scene/AuthForge/internal/health"
	"github.com/wreckx-in-scene/AuthForge/internal/serve"
	"github.com/wreckx-in-scene/AuthForge/services/session/internal/server"
	"github.com/wreckx-in-scene/AuthForge/services/session/internal/store"
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
	refreshTTL, err := config.Duration("REFRESH_TTL", "720h")
	if err != nil {
		return err
	}
	pool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	api := server.New(store.New(pool, refreshTTL), log)
	return serve.GRPC(ctx, ":"+config.String("HEALTH_PORT", "8081"), ":"+config.String("GRPC_PORT", "50051"), func(gs *grpc.Server) {
		sessionv1.RegisterSessionServiceServer(gs, api)
	})
}
