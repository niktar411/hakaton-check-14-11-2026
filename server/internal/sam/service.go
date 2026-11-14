package sam

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
)

const pageLimit int32 = 100

// Store is the read-only subset of sqlc queries used by SAM. Keeping it behind
// an interface lets the mapping and suggestion logic be tested without a database.
type Store interface {
	SamDashboard(ctx context.Context) (db.SamDashboardRow, error)
	SamPublicProducts(ctx context.Context, arg db.SamPublicProductsParams) ([]db.SamPublicProductsRow, error)
	ListSamLicenses(ctx context.Context, arg db.ListSamLicensesParams) ([]db.ListSamLicensesRow, error)
	ListSamAssets(ctx context.Context, arg db.ListSamAssetsParams) ([]db.ListSamAssetsRow, error)
	ListSamFindings(ctx context.Context, arg db.ListSamFindingsParams) ([]db.ListSamFindingsRow, error)
	ListSamMatches(ctx context.Context, arg db.ListSamMatchesParams) ([]db.ListSamMatchesRow, error)
}

// Service exposes the public read-only SAM projection.
type Service struct {
	store Store
}

func NewService(pool *pgxpool.Pool) *Service {
	return newService(db.New(pool))
}

func newService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Dashboard(ctx context.Context) (Overview, error) {
	row, err := s.store.SamDashboard(ctx)
	if err != nil {
		return Overview{}, fmt.Errorf("load SAM dashboard: %w", err)
	}
	return Overview{
		Products:                row.Products,
		Assets:                  row.Assets,
		Licenses:                row.Licenses,
		SeatsTotal:              row.SeatsTotal,
		SeatsUsed:               row.SeatsUsed,
		OpenFindings:            row.OpenFindings,
		PendingMatches:          row.PendingMatches,
		ExpiringLicenses:        row.ExpiringLicenses,
		UnlicensedInstallations: row.UnlicensedInstallations,
	}, nil
}

func (s *Service) Products(ctx context.Context) ([]Product, error) {
	rows, err := s.store.SamPublicProducts(ctx, db.SamPublicProductsParams{Limit: pageLimit})
	if err != nil {
		return nil, fmt.Errorf("list SAM products: %w", err)
	}
	products := make([]Product, len(rows))
	for i, row := range rows {
		products[i] = Product{
			ID:                row.ID,
			Name:              row.Name,
			Vendor:            row.Vendor,
			Category:          row.Category,
			SectionCode:       row.SectionCode,
			ClassCode:         row.ClassCode,
			LicenseMetric:     row.LicenseMetric,
			RegistryRf:        row.RegistryRf,
			RegistryEntry:     row.RegistryEntry,
			IsTrackable:       row.IsTrackable,
			LicenseCount:      row.LicenseCount,
			InstallationCount: row.InstallationCount,
			UpdatedAt:         row.UpdatedAt.Time,
		}
	}
	return products, nil
}

func (s *Service) Licenses(ctx context.Context) ([]License, error) {
	rows, err := s.store.ListSamLicenses(ctx, db.ListSamLicensesParams{Limit: pageLimit})
	if err != nil {
		return nil, fmt.Errorf("list SAM licenses: %w", err)
	}
	licenses := make([]License, len(rows))
	for i, row := range rows {
		licenses[i] = License{
			ID:         row.ID,
			Product:    row.ProductName,
			Vendor:     row.ProductVendor,
			Kind:       row.Kind,
			SeatsTotal: row.SeatsTotal,
			SeatsUsed:  row.SeatsUsed,
			Contract:   row.Contract,
			ValidUntil: optionalDate(row.ValidUntil),
			Status:     row.Status,
		}
	}
	return licenses, nil
}

func (s *Service) Assets(ctx context.Context) ([]Asset, error) {
	rows, err := s.store.ListSamAssets(ctx, db.ListSamAssetsParams{Limit: pageLimit})
	if err != nil {
		return nil, fmt.Errorf("list SAM assets: %w", err)
	}
	assets := make([]Asset, len(rows))
	for i, row := range rows {
		assets[i] = Asset{
			ID:            row.ID,
			Hostname:      row.Hostname,
			Platform:      row.Platform,
			Owner:         row.Owner,
			SoftwareCount: row.SoftwareCount,
			LastSeenAt:    optionalTimestamp(row.LastSeenAt),
		}
	}
	return assets, nil
}

func (s *Service) Findings(ctx context.Context) ([]Finding, error) {
	rows, err := s.store.ListSamFindings(ctx, db.ListSamFindingsParams{Limit: pageLimit})
	if err != nil {
		return nil, fmt.Errorf("list SAM findings: %w", err)
	}
	findings := make([]Finding, len(rows))
	for i, row := range rows {
		findings[i] = Finding{
			ID:         row.ID,
			Code:       row.Code,
			Severity:   row.Severity,
			Status:     row.Status,
			Product:    row.ProductName,
			Asset:      row.AssetHostname,
			Details:    row.Details,
			DetectedAt: row.DetectedAt.Time,
		}
	}
	return findings, nil
}

func (s *Service) Matches(ctx context.Context) ([]Match, error) {
	rows, err := s.store.ListSamMatches(ctx, db.ListSamMatchesParams{Limit: pageLimit})
	if err != nil {
		return nil, fmt.Errorf("list SAM matches: %w", err)
	}
	matches := make([]Match, len(rows))
	for i, row := range rows {
		matches[i] = Match{
			ID:           row.ID,
			DetectedName: row.DetectedName,
			Version:      row.Version,
			Asset:        row.AssetHostname,
			Product:      row.ProductName,
			Confidence:   row.Confidence,
			Status:       row.Status,
			CreatedAt:    row.CreatedAt.Time,
		}
	}
	return matches, nil
}

func (s *Service) Assistant(ctx context.Context) (Assistant, error) {
	dashboard, err := s.Dashboard(ctx)
	if err != nil {
		return Assistant{}, err
	}
	suggestions := make([]Suggestion, 0, 4)
	add := func(kind string, count int64, path string) {
		if count > 0 {
			suggestions = append(suggestions, Suggestion{Kind: kind, Count: count, Path: path})
		}
	}
	add("expiring_licenses", dashboard.ExpiringLicenses, "/sam/licenses")
	add("open_findings", dashboard.OpenFindings, "/sam/compliance")
	add("pending_matches", dashboard.PendingMatches, "/sam/matching")
	add("unlicensed_installations", dashboard.UnlicensedInstallations, "/sam/matching")
	return Assistant{Suggestions: suggestions}, nil
}

func optionalDate(value pgtype.Date) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.Format("2006-01-02")
	return &formatted
}

func optionalTimestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}
