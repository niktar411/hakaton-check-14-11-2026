package compliance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
)

// ErrNotFound is returned when a product or host does not exist.
var ErrNotFound = errors.New("sam compliance record not found")

// Store is the read-only subset of sqlc queries used by the compliance engine.
type Store interface {
	SamComplianceSummary(ctx context.Context) (db.SamComplianceSummaryRow, error)
	SamComplianceProducts(ctx context.Context) ([]db.SamComplianceProductsRow, error)
	SamComplianceProduct(ctx context.Context, id int64) (db.SamComplianceProductRow, error)
	ListSamProductLicenses(ctx context.Context, productID int64) ([]db.ListSamProductLicensesRow, error)
	ListSamProductHosts(ctx context.Context, productID int64) ([]db.ListSamProductHostsRow, error)
	GetSamHost(ctx context.Context, id int64) (db.GetSamHostRow, error)
	ListSamHostInstallations(ctx context.Context, assetID int64) ([]db.ListSamHostInstallationsRow, error)
}

// Service computes compliance balances and recommendations from the catalog.
type Service struct {
	store Store
}

func NewService(pool *pgxpool.Pool) *Service {
	return newService(db.New(pool))
}

func newService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Summary(ctx context.Context) (Summary, error) {
	row, err := s.store.SamComplianceSummary(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("load SAM compliance summary: %w", err)
	}
	products, err := s.products(ctx)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		Products:               row.Products,
		ActiveProducts:         row.ActiveProducts,
		BannedProducts:         row.BannedProducts,
		Assets:                 row.Assets,
		Licenses:               row.Licenses,
		Installations:          row.Installations,
		Devices:                row.Devices,
		UnmatchedInstallations: row.UnmatchedInstallations,
		PurchasedSeats:         row.PurchasedSeats,
		LicensedSeats:          row.LicensedSeats,
		ExpiredSeats:           row.ExpiredSeats,
		OverusedSeats:          row.OverusedSeats,
		IdleSeats:              row.IdleSeats,
		BannedInstallations:    row.BannedInstallations,
		AnnualCost:             row.AnnualCost,
		OveruseCost:            row.OveruseCost,
		IdleCost:               row.IdleCost,
		ExpiredCost:            row.ExpiredCost,
		CostAtRisk:             row.OveruseCost + row.ExpiredCost,
	}
	summary.CompliancePercent = compliancePercent(products)
	summary.RiskLevel = summaryRiskLevel(summary)
	summary.Recommendations = summaryRecommendations(summary)
	summary.Categories = categoryBreakdowns(products)
	summary.Statuses = statusBreakdowns(products)
	return summary, nil
}

func (s *Service) Products(ctx context.Context) ([]ComplianceProduct, error) {
	return s.products(ctx)
}

func (s *Service) products(ctx context.Context) ([]ComplianceProduct, error) {
	rows, err := s.store.SamComplianceProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load SAM compliance products: %w", err)
	}
	result := make([]ComplianceProduct, 0, len(rows))
	for _, row := range rows {
		result = append(result, productFromRow(row))
	}
	return result, nil
}

func (s *Service) Risks(ctx context.Context) ([]Risk, error) {
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

func (s *Service) Product(ctx context.Context, id int64) (ComplianceProductDetail, error) {
	row, err := s.store.SamComplianceProduct(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ComplianceProductDetail{}, fmt.Errorf("load SAM product %d: %w", id, ErrNotFound)
		}
		return ComplianceProductDetail{}, fmt.Errorf("load SAM product %d: %w", id, err)
	}
	licenseRows, err := s.store.ListSamProductLicenses(ctx, id)
	if err != nil {
		return ComplianceProductDetail{}, fmt.Errorf("load SAM product licences %d: %w", id, err)
	}
	hostRows, err := s.store.ListSamProductHosts(ctx, id)
	if err != nil {
		return ComplianceProductDetail{}, fmt.Errorf("load SAM product hosts %d: %w", id, err)
	}
	product := productFromRow(db.SamComplianceProductsRow(row))
	licenses := make([]ComplianceLicense, 0, len(licenseRows))
	for _, license := range licenseRows {
		licenses = append(licenses, ComplianceLicense{
			ID:          license.ID,
			Kind:        license.Kind,
			SeatsTotal:  license.SeatsTotal,
			SeatsUsed:   license.SeatsUsed,
			CostPerSeat: license.CostPerSeat,
			Currency:    license.Currency,
			ValidUntil:  optionalDate(license.ValidUntil),
			Status:      license.Status,
		})
	}
	hosts := make([]HostUsage, 0, len(hostRows))
	for _, host := range hostRows {
		hosts = append(hosts, HostUsage{
			AssetID:       host.AssetID,
			Hostname:      host.Hostname,
			Platform:      host.Platform,
			Owner:         host.Owner,
			Version:       host.Version,
			LastSeenAt:    optionalTimestamp(host.LastSeenAt),
			LicenseStatus: licenseStatus(host.ProductStatus, host.HasActiveLicense, host.HasExpiredLicense),
		})
	}
	return ComplianceProductDetail{Product: product, Licenses: licenses, Hosts: hosts}, nil
}

func (s *Service) Host(ctx context.Context, id int64) (HostDetail, error) {
	row, err := s.store.GetSamHost(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HostDetail{}, fmt.Errorf("load SAM host %d: %w", id, ErrNotFound)
		}
		return HostDetail{}, fmt.Errorf("load SAM host %d: %w", id, err)
	}
	installationRows, err := s.store.ListSamHostInstallations(ctx, id)
	if err != nil {
		return HostDetail{}, fmt.Errorf("load SAM host installations %d: %w", id, err)
	}
	detail := HostDetail{
		ID:         row.ID,
		Hostname:   row.Hostname,
		Platform:   row.Platform,
		Owner:      row.Owner,
		LastSeenAt: optionalTimestamp(row.LastSeenAt),
	}
	detail.Installations = make([]HostInstallation, 0, len(installationRows))
	for _, installation := range installationRows {
		productID := optionalInt64(installation.ProductID)
		productName := installation.DetectedName
		status := "unlicensed"
		if installation.ProductID.Valid {
			productName = installation.ProductName.String
			status = licenseStatus(installation.ProductStatus.String, installation.HasActiveLicense, installation.HasExpiredLicense)
		}
		detail.Installations = append(detail.Installations, HostInstallation{
			ID:           installation.ID,
			ProductID:    productID,
			Product:      productName,
			Vendor:       installation.ProductVendor.String,
			Category:     installation.ProductCategory.String,
			DetectedName: installation.DetectedName,
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

func productFromRow(row db.SamComplianceProductsRow) ComplianceProduct {
	product := ComplianceProduct{
		ID:                  row.ID,
		Name:                row.Name,
		Vendor:              row.Vendor,
		Category:            row.Category,
		Status:              row.Status,
		Licenses:            row.Licenses,
		PurchasedSeats:      row.PurchasedSeats,
		LicensedSeats:       row.LicensedSeats,
		ExpiredSeats:        row.ExpiredSeats,
		InstalledDevices:    row.InstalledDevices,
		Installations:       row.Installations,
		BannedInstallations: row.BannedInstallations,
		AnnualCost:          row.AnnualCost,
		AvgCostPerSeat:      row.AvgCostPerSeat,
	}
	product.DeltaSeats = product.InstalledDevices - product.LicensedSeats
	if product.Status == "banned" {
		product.BannedInstallations = product.InstalledDevices
		product.LicensedSeats = 0
		product.ExpiredSeats = 0
		product.DeltaSeats = product.BannedInstallations
	} else {
		product.BannedInstallations = 0
		product.OverusedSeats = max(product.InstalledDevices-product.LicensedSeats, 0)
		product.IdleSeats = max(product.LicensedSeats-product.InstalledDevices, 0)
		product.RiskCost = (product.OverusedSeats + product.ExpiredSeats + product.IdleSeats) * product.AvgCostPerSeat
	}
	product.RiskLevel = productRiskLevel(product)
	product.Recommendation = productRecommendation(product)
	return product
}

func summaryRiskLevel(summary Summary) string {
	if summary.OverusedSeats > 0 || summary.ExpiredSeats > 0 || summary.BannedInstallations > 0 {
		return "high"
	}
	if summary.IdleSeats > 0 || summary.UnmatchedInstallations > 0 {
		return "medium"
	}
	return "low"
}

func productRiskLevel(product ComplianceProduct) string {
	if product.Status == "banned" && product.BannedInstallations > 0 {
		return "high"
	}
	if product.OverusedSeats > 0 || product.ExpiredSeats > 0 {
		return "high"
	}
	if product.IdleSeats > 0 || ((product.Status == "inactive" || product.Status == "archived") && product.InstalledDevices > 0) {
		return "medium"
	}
	return "low"
}

func productRecommendation(product ComplianceProduct) string {
	switch {
	case product.Status == "banned" && product.BannedInstallations > 0:
		return fmt.Sprintf("Удалить запрещённое ПО с %d %s.", product.BannedInstallations, plural(product.BannedInstallations, "устройства", "устройств", "устройств"))
	case product.ExpiredSeats > 0:
		return fmt.Sprintf("Продлить или заменить %d %s.", product.ExpiredSeats, plural(product.ExpiredSeats, "просроченное место", "просроченных места", "просроченных мест"))
	case product.OverusedSeats > 0:
		return fmt.Sprintf("Докупить %d %s или удалить лишние установки.", product.OverusedSeats, plural(product.OverusedSeats, "место", "места", "мест"))
	case product.IdleSeats > 0:
		return fmt.Sprintf("Вернуть или перераспределить %d %s.", product.IdleSeats, plural(product.IdleSeats, "неиспользуемое место", "неиспользуемых места", "неиспользуемых мест"))
	default:
		return "Действий не требуется."
	}
}

func summaryRecommendations(summary Summary) []string {
	result := make([]string, 0, 5)
	if summary.BannedInstallations > 0 {
		result = append(result, fmt.Sprintf("Удалить запрещённое ПО с %d %s.", summary.BannedInstallations, plural(summary.BannedInstallations, "устройства", "устройств", "устройств")))
	}
	if summary.OverusedSeats > 0 {
		result = append(result, fmt.Sprintf("Докупить %d %s или удалить лишние установки.", summary.OverusedSeats, plural(summary.OverusedSeats, "место", "места", "мест")))
	}
	if summary.ExpiredSeats > 0 {
		result = append(result, fmt.Sprintf("Продлить или заменить %d %s.", summary.ExpiredSeats, plural(summary.ExpiredSeats, "просроченное место", "просроченных места", "просроченных мест")))
	}
	if summary.IdleSeats > 0 {
		result = append(result, fmt.Sprintf("Вернуть или перераспределить %d %s.", summary.IdleSeats, plural(summary.IdleSeats, "неиспользуемое место", "неиспользуемых места", "неиспользуемых мест")))
	}
	if summary.UnmatchedInstallations > 0 {
		result = append(result, fmt.Sprintf("Сопоставить %d %s с каталогом.", summary.UnmatchedInstallations, plural(summary.UnmatchedInstallations, "установку", "установки", "установок")))
	}
	if len(result) == 0 {
		result = append(result, "Критических действий не требуется.")
	}
	return result
}

func compliancePercent(products []ComplianceProduct) int64 {
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

func categoryBreakdowns(products []ComplianceProduct) []CategoryBreakdown {
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

func statusBreakdowns(products []ComplianceProduct) []StatusBreakdown {
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

func buildRisks(summary Summary, products []ComplianceProduct) []Risk {
	risks := make([]Risk, 0, len(products)+1)
	for _, product := range products {
		id := product.ID
		name := product.Name
		if product.Status == "banned" && product.BannedInstallations > 0 {
			risks = append(risks, Risk{
				Code: "banned_product", Severity: "high", ProductID: &id, ProductName: name,
				Category: product.Category, Affected: product.BannedInstallations,
				Recommendation: fmt.Sprintf("Удалить запрещённое ПО с %d %s.", product.BannedInstallations, plural(product.BannedInstallations, "устройства", "устройств", "устройств")),
			})
		}
		if product.OverusedSeats > 0 {
			risks = append(risks, Risk{
				Code: "license_overuse", Severity: "high", ProductID: &id, ProductName: name,
				Category: product.Category, Affected: product.OverusedSeats,
				Cost:           product.OverusedSeats * product.AvgCostPerSeat,
				Recommendation: fmt.Sprintf("Докупить %d %s или удалить лишние установки.", product.OverusedSeats, plural(product.OverusedSeats, "место", "места", "мест")),
			})
		}
		if product.ExpiredSeats > 0 {
			risks = append(risks, Risk{
				Code: "expired_license", Severity: "high", ProductID: &id, ProductName: name,
				Category: product.Category, Affected: product.ExpiredSeats,
				Cost:           product.ExpiredSeats * product.AvgCostPerSeat,
				Recommendation: fmt.Sprintf("Продлить или заменить %d %s.", product.ExpiredSeats, plural(product.ExpiredSeats, "просроченное место", "просроченных места", "просроченных мест")),
			})
		}
		if product.IdleSeats > 0 {
			risks = append(risks, Risk{
				Code: "idle_license", Severity: "medium", ProductID: &id, ProductName: name,
				Category: product.Category, Affected: product.IdleSeats,
				Cost:           product.IdleSeats * product.AvgCostPerSeat,
				Recommendation: fmt.Sprintf("Вернуть или перераспределить %d %s.", product.IdleSeats, plural(product.IdleSeats, "неиспользуемое место", "неиспользуемых места", "неиспользуемых мест")),
			})
		}
		if (product.Status == "inactive" || product.Status == "archived") && product.InstalledDevices > 0 {
			risks = append(risks, Risk{
				Code: "inactive_product", Severity: "medium", ProductID: &id, ProductName: name,
				Category: product.Category, Affected: product.InstalledDevices,
				Recommendation: fmt.Sprintf("Перевести продукт в актуальный статус или удалить %d %s.", product.InstalledDevices, plural(product.InstalledDevices, "установку", "установки", "установок")),
			})
		}
	}
	if summary.UnmatchedInstallations > 0 {
		risks = append(risks, Risk{
			Code: "unmatched_installations", Severity: "high",
			Affected:       summary.UnmatchedInstallations,
			Recommendation: fmt.Sprintf("Сопоставить %d %s с каталогом.", summary.UnmatchedInstallations, plural(summary.UnmatchedInstallations, "установку", "установки", "установок")),
		})
	}
	sort.SliceStable(risks, func(i, j int) bool {
		if risks[i].Severity != risks[j].Severity {
			return risks[i].Severity == "high"
		}
		if risks[i].Cost != risks[j].Cost {
			return risks[i].Cost > risks[j].Cost
		}
		if risks[i].Affected != risks[j].Affected {
			return risks[i].Affected > risks[j].Affected
		}
		return risks[i].ProductName < risks[j].ProductName
	})
	return risks
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

func plural(value int64, one, few, many string) string {
	remainder := value % 100
	if remainder >= 11 && remainder <= 14 {
		return many
	}
	switch value % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
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
