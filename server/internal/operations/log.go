package operations

import (
	"context"
	"log/slog"
)

func (s *Service) logUsageFailure(ctx context.Context, id int64) {
	slog.ErrorContext(ctx, "usage completion failed; pending record retained", "id", id)
}
func (s *Service) logStorageFailure(ctx context.Context) {
	slog.ErrorContext(ctx, "transaction rollback failed")
}
