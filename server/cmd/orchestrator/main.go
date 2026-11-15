package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hackathon/internal/infra/orchestrator"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.ErrorContext(ctx, "orchestrator stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	server, err := orchestrator.NewServer(orchestrator.Config{
		Token:       os.Getenv("ORCHESTRATOR_TOKEN"),
		WorkerToken: os.Getenv("WORKER_TOKEN"),
		Upstream:    os.Getenv("WORKER_URL"),
	})
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              envOr("ORCHESTRATOR_BIND", "0.0.0.0:8080"),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      130 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16384,
	}
	done := make(chan error, 1)
	go func() { done <- httpServer.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdown); err != nil {
			return err
		}
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
