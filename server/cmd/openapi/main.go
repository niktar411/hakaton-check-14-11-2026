package main

import (
	"context"
	"hackathon/internal/operations"
	"log/slog"
	"os"
)

func main() {
	ctx := context.Background()
	if err := operations.WriteOpenAPI(ctx, "../api/openapi.json"); err != nil {
		slog.ErrorContext(ctx, "API generation failed", "error", err)
		os.Exit(1)
	}
}
