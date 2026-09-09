package config

import (
	"fmt"
	"os"
)

type Config struct {
	GRPCPort    string
	DatabaseURL string

	// PlatformJWTSecret signs tokens for platform accounts (AccountLogin) —
	// this has no per-project context, so it lives in env config, not the DB.
	// Tenant-user tokens use each project's own jwt_secret column instead.
	PlatformJWTSecret string
}

func Load() (*Config, error) {
	cfg := &Config{
		GRPCPort:          getEnv("GRPC_PORT", "50051"),
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		PlatformJWTSecret: getEnv("PLATFORM_JWT_SECRET", ""),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.PlatformJWTSecret == "" {
		return nil, fmt.Errorf("PLATFORM_JWT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
