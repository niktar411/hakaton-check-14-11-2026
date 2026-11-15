package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"hackathon/internal/infra/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.ErrorContext(ctx, "worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	config := worker.ConfigFromEnv()
	instance, err := worker.NewWorker(config)
	if err != nil {
		return err
	}
	if err := instance.Bootstrap(); err != nil {
		return err
	}
	agent, err := worker.NewAgent(worker.AgentConfig{
		Token:    os.Getenv("WORKER_TOKEN"),
		CallerIP: envOr("WORKER_CALLER_IP", "10.77.0.1"),
		BindIP:   envOr("WORKER_BIND_IP", "10.77.0.10"),
	}, instance)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(envOr("WORKER_PORT", "9102"))
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              envOr("WORKER_BIND_IP", "10.77.0.10") + ":" + strconv.Itoa(port),
		Handler:           agent.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      130 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16384,
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
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
