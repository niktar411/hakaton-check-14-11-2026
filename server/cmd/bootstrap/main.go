package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"hackathon/internal/localaccounts"
)

type bootstrapConfig struct {
	databaseURL string
	username    string
	password    string
	color       string
}

func loadConfig(getenv func(string) string) (bootstrapConfig, error) {
	config := bootstrapConfig{
		databaseURL: getenv("DATABASE_URL"),
		username:    getenv("ADMIN_USERNAME"),
		password:    getenv("ADMIN_PASSWORD"),
		color:       getenv("ADMIN_COLOR"),
	}
	for _, required := range []struct {
		name  string
		value string
	}{
		{name: "DATABASE_URL", value: config.databaseURL},
		{name: "ADMIN_USERNAME", value: config.username},
		{name: "ADMIN_PASSWORD", value: config.password},
		{name: "ADMIN_COLOR", value: config.color},
	} {
		if required.value == "" {
			return bootstrapConfig{}, fmt.Errorf("missing configuration: %s", required.name)
		}
	}
	return config, nil
}

func run(ctx context.Context, getenv func(string) string) error {
	config, err := loadConfig(getenv)
	if err != nil {
		return fmt.Errorf("bootstrap configuration: %w", err)
	}
	poolConfig, err := pgxpool.ParseConfig(config.databaseURL)
	if err != nil {
		return fmt.Errorf("parse bootstrap database configuration: %w", err)
	}
	poolConfig.MaxConns = 1
	poolConfig.MinConns = 0

	operationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(operationCtx, poolConfig)
	if err != nil {
		return fmt.Errorf("connect bootstrap database: %w", err)
	}
	defer pool.Close()

	account, err := localaccounts.BootstrapAdmin(operationCtx, pool, config.username, config.password, config.color)
	if err != nil {
		return fmt.Errorf("initialize administrator: %w", err)
	}
	slog.InfoContext(operationCtx, "initial administrator ready", "account_id", account.ID)
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv); err != nil {
		// Database and hashing errors can contain connection details. Keep logs
		// deliberately free of credentials and environment values.
		slog.ErrorContext(ctx, "bootstrap failed")
		os.Exit(1)
	}
}
