package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	authv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/auth/v1"
	tenantv1 "github.com/wreckx-in-scene/AuthForge/gen/authforge/tenant/v1"
	"github.com/wreckx-in-scene/AuthForge/internal/config"
	"github.com/wreckx-in-scene/AuthForge/internal/health"
	"github.com/wreckx-in-scene/AuthForge/services/gateway/internal/httpapi"
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
	tenantAddr, err := config.Require("TENANT_GRPC_ADDR")
	if err != nil {
		return err
	}
	authAddr, err := config.Require("AUTH_GRPC_ADDR")
	if err != nil {
		return err
	}
	issuer, err := config.Require("ISSUER")
	if err != nil {
		return err
	}
	secret, err := config.Require("CONSOLE_TOKEN_SECRET")
	if err != nil {
		return err
	}
	if len(secret) < 32 {
		return errString("CONSOLE_TOKEN_SECRET must be at least 32 bytes")
	}
	tenantConn, err := grpc.NewClient(tenantAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer tenantConn.Close()
	authConn, err := grpc.NewClient(authAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer authConn.Close()

	authClient := authv1.NewAuthServiceClient(authConn)
	verifier, err := httpapi.LoadVerifier(ctx, authClient, issuer)
	if err != nil {
		return err
	}
	api := httpapi.New(tenantv1.NewTenantServiceClient(tenantConn), authClient, verifier, []byte(secret), issuer, log)
	httpServer := &http.Server{
		Addr:              ":" + config.String("HTTP_PORT", "8080"),
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

type errString string

func (e errString) Error() string { return string(e) }
