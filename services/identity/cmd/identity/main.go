package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"google.golang.org/grpc"

	identityv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/identity/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/config"
	"github.com/wreckx-in-scene/AuthForge/internal/database"
	"github.com/wreckx-in-scene/AuthForge/internal/health"
	"github.com/wreckx-in-scene/AuthForge/internal/mailer"
	"github.com/wreckx-in-scene/AuthForge/internal/serve"
	"github.com/wreckx-in-scene/AuthForge/services/identity/internal/server"
	"github.com/wreckx-in-scene/AuthForge/services/identity/internal/store"
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
	smtpAddr, err := config.Require("SMTP_ADDR")
	if err != nil {
		return err
	}
	smtpFrom, err := config.Require("SMTP_FROM")
	if err != nil {
		return err
	}
	verificationURL, err := config.Require("EMAIL_VERIFICATION_URL")
	if err != nil {
		return err
	}
	if _, err := mailer.EmailVerificationURL(verificationURL, "startup-validation", "startup-validation"); err != nil {
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
	api := server.New(store.New(pool), verificationURL, smtp, log)
	return serve.GRPC(ctx, ":"+config.String("HEALTH_PORT", "8081"), ":"+config.String("GRPC_PORT", "50051"), func(gs *grpc.Server) {
		identityv1.RegisterIdentityServiceServer(gs, api)
	})
}

type errString string

func (e errString) Error() string { return string(e) }
