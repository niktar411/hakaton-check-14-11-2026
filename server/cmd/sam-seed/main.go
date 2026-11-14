package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"hackathon/internal/sam/catalog"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.ErrorContext(ctx, "sam seed failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	input := flag.String("in", "demo/sam/catalog.json", "catalog.json produced by the catalog task")
	output := flag.String("out", "", "write the normalized seed to this file instead of stdout")
	flag.Parse()

	result, err := catalog.LoadFile(ctx, *input)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	for _, issue := range result.Issues {
		slog.WarnContext(ctx, "catalog issue", "index", issue.Index, "field", issue.Field, "code", issue.Code, "message", issue.Message)
	}
	data, err := json.MarshalIndent(result.Seed, "", "  ")
	if err != nil {
		return fmt.Errorf("encode seed: %w", err)
	}
	data = append(data, '\n')
	if *output == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("write seed: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		return fmt.Errorf("create seed directory: %w", err)
	}
	if err := os.WriteFile(*output, data, 0644); err != nil {
		return fmt.Errorf("write seed %s: %w", *output, err)
	}
	slog.InfoContext(ctx, "seed written", "path", *output, "products", len(result.Seed.Products), "licenses", len(result.Seed.Licenses), "issues", len(result.Issues))
	return nil
}
