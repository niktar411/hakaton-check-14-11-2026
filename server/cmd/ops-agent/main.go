package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"hackathon/internal/infra/ops"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.ErrorContext(ctx, "control agent stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	callers := map[string]bool{}
	list := envOr("AGENT_CALLERS", envOr("AGENT_CALLER", "10.77.0.1,10.77.0.10,172.30.0.5"))
	for _, value := range strings.Split(list, ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			callers[trimmed] = true
		}
	}
	vpn := ops.NewVPN(ops.VPNConfig{
		ConfigPath: envOr("AWG_CONFIG", "/etc/amnezia/amneziawg/awg0.conf"),
		Profiles:   envOr("AWG_PROFILES", "/home/perdun/team-access"),
		StatePath:  envOr("AGENT_STATE", "/var/lib/hackathon-agent/state.json"),
	})
	if err := vpn.RecoverTransaction(); err != nil {
		return err
	}
	if _, err := vpn.LoadState(); err != nil {
		return err
	}
	agent, err := ops.NewAgent(ops.AgentConfig{
		Token:       os.Getenv("AGENT_TOKEN"),
		Callers:     callers,
		WorkerURL:   envOr("WORKER_URL", "http://10.77.0.10:9102"),
		WorkerToken: os.Getenv("WORKER_TOKEN"),
		VPN:         vpn,
		Production:  ops.ProductionFromEnv,
	})
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              envOr("AGENT_BIND", "10.77.0.1:9101"),
		Handler:           agent.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      100 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8192,
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
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
