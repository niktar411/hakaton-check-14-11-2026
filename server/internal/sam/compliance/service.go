package compliance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
)

// ErrNotFound is returned when a product or host does not exist.
var ErrNotFound = errors.New("sam compliance record not found")

type Store interface {
	SamComplianceSummary(ctx context.Context) (db.SamComplianceSummaryRow, error)
	SamComplianceProducts(ctx context.Context) ([]db.SamComplianceProductsRow, error)
	SamComplianceProduct(ctx context.Context, id int64) (db.SamComplianceProductRow, error)
	ListSamProductLicenses(ctx context.Context, productID int64) ([]db.ListSamProductLicensesRow, error)
	ListSamProductHosts(ctx context.Context, productID int64) ([]db.ListSamProductHostsRow, error)
	GetSamHost(ctx context.Context, hostname string) (db.GetSamHostRow, error)
	ListSamHostInstallations(ctx context.Context, hostname string) ([]db.ListSamHostInstallationsRow, error)
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
	products, err := s.products(ctx)
	if err != nil {
		return SamComplianceSummary{}, err
	}
	summary := summaryFromRow(row)
	summary.CompliancePercent = compliancePercent(products)
	summary.Categories = categoryBreakdowns(products)
	summary.Statuses = statusBreakdowns(products)
	return summary, nil
}

func (s *Service) Products(ctx context.Context) ([]SamComplianceProduct, error) {
	return s.products(ctx)
}

func (s *Service) products(ctx context.Context) ([]SamComplianceProduct, error) {
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

func (s *Service) Product(ctx context.Context, id int64) (SamComplianceProductDetail, error) {
	row, err := s.store.SamComplianceProduct(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SamComplianceProductDetail{}, fmt.Errorf("load SAM product %d: %w", id, ErrNotFound)
		}
		return SamComplianceProductDetail{}, storageError(ctx, fmt.Sprintf("load SAM product %d", id), err)
	}
	licenseRows, err := s.store.ListSamProductLicenses(ctx, id)
	if err != nil {
		return SamComplianceProductDetail{}, storageError(ctx, fmt.Sprintf("load SAM product licences %d", id), err)
	}
	hostRows, err := s.store.ListSamProductHosts(ctx, id)
	if err != nil {
		return SamComplianceProductDetail{}, storageError(ctx, fmt.Sprintf("load SAM product hosts %d", id), err)
	}
	licenses := make([]SamComplianceLicense, 0, len(licenseRows))
	for _, license := range licenseRows {
		licenses = append(licenses, SamComplianceLicense{
			ID:          license.ID,
			Kind:        license.LicenseType,
			SeatsTotal:  license.Seats,
			CostPerSeat: license.CostPerSeat,
			Currency:    license.Currency,
			ValidUntil:  optionalDate(license.ExpiresAt),
			Status:      license.Status,
		})
	}
	hosts := make([]SamComplianceHostUsage, 0, len(hostRows))
	for _, host := range hostRows {
		hosts = append(hosts, SamComplianceHostUsage{
			Hostname:      host.DeviceName,
			Version:       host.Version,
			LastSeenAt:    optionalTimestamp(host.LastSeenAt),
			LicenseStatus: licenseStatus(host.ProductStatus, host.HasActiveLicense, host.HasExpiredLicense),
		})
	}
	return SamComplianceProductDetail{
		Product:  productFromRow(db.SamComplianceProductsRow(row)),
		Licenses: licenses,
		Hosts:    hosts,
	}, nil
}

func (s *Service) Host(ctx context.Context, hostname string) (SamComplianceHostDetail, error) {
	row, err := s.store.GetSamHost(ctx, hostname)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SamComplianceHostDetail{}, fmt.Errorf("load SAM host %s: %w", hostname, ErrNotFound)
		}
		return SamComplianceHostDetail{}, storageError(ctx, fmt.Sprintf("load SAM host %s", hostname), err)
	}
	installationRows, err := s.store.ListSamHostInstallations(ctx, hostname)
	if err != nil {
		return SamComplianceHostDetail{}, storageError(ctx, fmt.Sprintf("load SAM host installations %s", hostname), err)
	}
	detail := SamComplianceHostDetail{
		Hostname:   row.DeviceName,
		LastSeenAt: optionalTimestamp(row.LastSeenAt),
	}
	detail.Installations = make([]SamComplianceHostInstallation, 0, len(installationRows))
	for _, installation := range installationRows {
		productID := optionalInt64(installation.ProductID)
		productName := installation.RawName
		status := "unlicensed"
		if installation.ProductID.Valid {
			productName = installation.ProductName.String
			status = licenseStatus(installation.ProductStatus.String, installation.HasActiveLicense, installation.HasExpiredLicense)
		}
		detail.Installations = append(detail.Installations, SamComplianceHostInstallation{
			ID:           installation.ID,
			ProductID:    productID,
			Product:      productName,
			Vendor:       installation.ProductVendor.String,
			Category:     installation.ProductCategory.String,
			DetectedName: installation.RawName,
			Version:      installation.Version,
			Status:       status,
		})
		switch status {
		case "licensed":
			detail.Licensed++
		case "expired":
			detail.Expired++
		case "banned":
			detail.Banned++
		case "inactive":
			detail.Inactive++
		default:
			detail.Unlicensed++
		}
	}
	return detail, nil
}

func storageError(ctx context.Context, operation string, err error) error {
	wrapped := fmt.Errorf("%s: %w", operation, err)
	slog.ErrorContext(ctx, "SAM compliance storage operation failed", "error", wrapped)
	return huma.Error503ServiceUnavailable("storage.unavailable")
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

func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	number := value.Int64
	return &number
}

func licenseStatus(productStatus string, hasActive, hasExpired bool) string {
	if productStatus == "banned" {
		return "banned"
	}
	if hasActive {
		return "licensed"
	}
	if hasExpired {
		return "expired"
	}
	if productStatus == "inactive" || productStatus == "archived" {
		return "inactive"
	}
	return "unlicensed"
}

func compliancePercent(products []SamComplianceProduct) int64 {
	if len(products) == 0 {
		return 0
	}
	violations := 0
	for _, product := range products {
		if product.OverusedSeats > 0 ||
			product.ExpiredSeats > 0 ||
			product.BannedInstallations > 0 ||
			((product.Status == "inactive" || product.Status == "archived") && product.InstalledDevices > 0) {
			violations++
		}
	}
	return int64(math.Round(float64(len(products)-violations) * 100 / float64(len(products))))
}

func categoryBreakdowns(products []SamComplianceProduct) []CategoryBreakdown {
	byName := map[string]*CategoryBreakdown{}
	for _, product := range products {
		category := product.Category
		if category == "" {
			category = "—"
		}
		item := byName[category]
		if item == nil {
			item = &CategoryBreakdown{Category: category}
			byName[category] = item
		}
		item.Products++
		item.InstalledDevices += product.InstalledDevices
		item.OverusedSeats += product.OverusedSeats
		item.RiskCost += product.RiskCost
	}
	result := make([]CategoryBreakdown, 0, len(byName))
	for _, item := range byName {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].InstalledDevices != result[j].InstalledDevices {
			return result[i].InstalledDevices > result[j].InstalledDevices
		}
		if result[i].RiskCost != result[j].RiskCost {
			return result[i].RiskCost > result[j].RiskCost
		}
		return result[i].Category < result[j].Category
	})
	return result
}

func statusBreakdowns(products []SamComplianceProduct) []StatusBreakdown {
	order := []string{"active", "inactive", "archived", "banned"}
	byStatus := map[string]*StatusBreakdown{}
	for _, status := range order {
		byStatus[status] = &StatusBreakdown{Status: status}
	}
	for _, product := range products {
		item := byStatus[product.Status]
		if item == nil {
			item = &StatusBreakdown{Status: product.Status}
			byStatus[product.Status] = item
			order = append(order, product.Status)
		}
		item.Products++
		item.InstalledDevices += product.InstalledDevices
	}
	result := make([]StatusBreakdown, 0, len(order))
	for _, status := range order {
		result = append(result, *byStatus[status])
	}
	return result
}
