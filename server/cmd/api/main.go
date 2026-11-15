package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"hackathon/internal/operations"
	"hackathon/migrations"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func run(ctx context.Context) error {
	config, err := operations.LoadConfig()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	migrationDB, err := sql.Open("pgx", config.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	migrationDB.SetMaxOpenConns(1)
	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("migration dialect: %w", err)
	}
	if err := goose.UpContext(ctx, migrationDB, "."); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := migrationDB.Close(); err != nil {
		return fmt.Errorf("close migration database: %w", err)
	}
	pgConfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database configuration: %w", err)
	}
	pgConfig.MaxConns = 4
	pgConfig.MinConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	service := operations.NewService(pool, config)
	server := &http.Server{Addr: config.Listen, Handler: service.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 150 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.ErrorContext(ctx, "portal stopped", "error", err)
		os.Exit(1)
	}
}
