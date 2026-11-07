package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"google.golang.org/grpc"

	tenantv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/tenant/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/config"
	"github.com/wreckx-in-scene/AuthForge/internal/database"
	"github.com/wreckx-in-scene/AuthForge/internal/health"
	"github.com/wreckx-in-scene/AuthForge/internal/mailer"
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
	smtpAddr, err := config.Require("SMTP_ADDR")
	if err != nil {
		return err
	}
	smtpFrom, err := config.Require("SMTP_FROM")
	if err != nil {
		return err
	}
	resetURL, err := config.Require("RESET_PASSWORD_URL")
	if err != nil {
		return err
	}
	if _, err := mailer.PasswordResetURL(resetURL, "startup-validation"); err != nil {
		return err
	}
	startTLS, err := strconv.ParseBool(config.String("SMTP_STARTTLS", "true"))
	if err != nil {
		return errString("SMTP_STARTTLS must be true or false")
	}
	smtp, err := mailer.NewSMTP(
		smtpAddr,
		smtpFrom,
		config.String("SMTP_USERNAME", ""),
		config.String("SMTP_PASSWORD", ""),
		startTLS,
	)
	if err != nil {
		return err
	}
	pool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	api := server.New(store.New(pool), []byte(secret), issuer, consoleTTL, resetURL, smtp, log)
	return serve.GRPC(ctx, ":"+config.String("HEALTH_PORT", "8081"), ":"+config.String("GRPC_PORT", "50051"), func(gs *grpc.Server) {
		tenantv1.RegisterTenantServiceServer(gs, api)
	})
}

type errString string

func (e errString) Error() string { return string(e) }
