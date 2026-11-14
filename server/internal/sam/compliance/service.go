package compliance

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
)

type Store interface {
	SamComplianceSummary(ctx context.Context) (db.SamComplianceSummaryRow, error)
	SamComplianceProducts(ctx context.Context) ([]db.SamComplianceProductsRow, error)
}

type Service struct {
	store Store
}

func NewService(pool *pgxpool.Pool) *Service {
	return newService(db.New(pool))
}

func newService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Summary(ctx context.Context) (SamComplianceSummary, error) {
	row, err := s.store.SamComplianceSummary(ctx)
	if err != nil {
		return SamComplianceSummary{}, storageError(ctx, "load SAM compliance summary", err)
	}
	return summaryFromRow(row), nil
}

func (s *Service) Products(ctx context.Context) ([]SamComplianceProduct, error) {
	rows, err := s.store.SamComplianceProducts(ctx)
	if err != nil {
		return nil, storageError(ctx, "load SAM compliance products", err)
	}
	result := make([]SamComplianceProduct, 0, len(rows))
	for _, row := range rows {
		result = append(result, productFromRow(row))
	}
	return result, nil
}

func (s *Service) Risks(ctx context.Context) ([]SamComplianceRisk, error) {
	summary, err := s.Summary(ctx)
	if err != nil {
		return nil, err
	}
	products, err := s.Products(ctx)
	if err != nil {
		return nil, err
	}
	return buildRisks(summary, products), nil
}

func storageError(ctx context.Context, operation string, err error) error {
	wrapped := fmt.Errorf("%s: %w", operation, err)
	slog.ErrorContext(ctx, "SAM compliance storage operation failed", "error", wrapped)
	return huma.Error503ServiceUnavailable("storage.unavailable")
}
