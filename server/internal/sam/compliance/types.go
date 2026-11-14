package compliance

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"hackathon/internal/db/sqlc"
)

type SamComplianceProduct struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Vendor              string `json:"vendor"`
	Category            string `json:"category"`
	Status              string `json:"status"`
	Licenses            int64  `json:"licenses"`
	PurchasedSeats      int64  `json:"purchasedSeats"`
	LicensedSeats       int64  `json:"licensedSeats"`
	ExpiredSeats        int64  `json:"expiredSeats"`
	InstalledDevices    int64  `json:"installedDevices"`
	Installations       int64  `json:"installations"`
	BannedInstallations int64  `json:"bannedInstallations"`
	OverusedSeats       int64  `json:"overusedSeats"`
	IdleSeats           int64  `json:"idleSeats"`
	AnnualCost          int64  `json:"annualCost"`
	AvgCostPerSeat      int64  `json:"avgCostPerSeat"`
	RiskCost            int64  `json:"riskCost"`
	RiskLevel           string `json:"riskLevel"`
	Recommendation      string `json:"recommendation"`
	DeltaSeats          int64  `json:"deltaSeats"`
}

type SamComplianceSummary struct {
	Products               int64               `json:"products"`
	ActiveProducts         int64               `json:"activeProducts"`
	BannedProducts         int64               `json:"bannedProducts"`
	Installations          int64               `json:"installations"`
	Devices                int64               `json:"devices"`
	UnmatchedInstallations int64               `json:"unmatchedInstallations"`
	LicensedSeats          int64               `json:"licensedSeats"`
	OverusedSeats          int64               `json:"overusedSeats"`
	IdleSeats              int64               `json:"idleSeats"`
	ExpiredSeats           int64               `json:"expiredSeats"`
	BannedInstallations    int64               `json:"bannedInstallations"`
	AnnualCost             int64               `json:"annualCost"`
	OveruseCost            int64               `json:"overuseCost"`
	IdleCost               int64               `json:"idleCost"`
	ExpiredCost            int64               `json:"expiredCost"`
	CostAtRisk             int64               `json:"costAtRisk"`
	Licenses               int64               `json:"licenses"`
	PurchasedSeats         int64               `json:"purchasedSeats"`
	CompliancePercent      int64               `json:"compliancePercent"`
	RiskLevel              string              `json:"riskLevel"`
	Recommendations        []string            `json:"recommendations"`
	Categories             []CategoryBreakdown `json:"categories"`
	Statuses               []StatusBreakdown   `json:"statuses"`
}

// CategoryBreakdown powers the category chart and its accessible table.
type CategoryBreakdown struct {
	Category         string `json:"category"`
	Products         int64  `json:"products"`
	InstalledDevices int64  `json:"installedDevices"`
	OverusedSeats    int64  `json:"overusedSeats"`
	RiskCost         int64  `json:"riskCost"`
}

// StatusBreakdown powers the catalog status chart.
type StatusBreakdown struct {
	Status           string `json:"status"`
	Products         int64  `json:"products"`
	InstalledDevices int64  `json:"installedDevices"`
}

// SamComplianceLicense is a license record shown on the product detail screen.
type SamComplianceLicense struct {
	ID          int64   `json:"id"`
	Kind        string  `json:"kind"`
	SeatsTotal  int64   `json:"seatsTotal"`
	SeatsUsed   int64   `json:"seatsUsed"`
	CostPerSeat int64   `json:"costPerSeat"`
	Currency    string  `json:"currency"`
	ValidUntil  *string `json:"validUntil"`
	Status      string  `json:"status"`
}

// SamComplianceHostUsage links a product to one affected host.
type SamComplianceHostUsage struct {
	Hostname      string     `json:"hostname"`
	Version       string     `json:"version"`
	LastSeenAt    *time.Time `json:"lastSeenAt"`
	LicenseStatus string     `json:"licenseStatus"`
}

// SamComplianceProductDetail is the drill-down target for a compliance row.
type SamComplianceProductDetail struct {
	Product  SamComplianceProduct     `json:"product"`
	Licenses []SamComplianceLicense   `json:"licenses"`
	Hosts    []SamComplianceHostUsage `json:"hosts"`
}

// SamComplianceHostInstallation is one discovered installation on a host screen.
type SamComplianceHostInstallation struct {
	ID           int64  `json:"id"`
	ProductID    *int64 `json:"productId,omitempty"`
	Product      string `json:"product"`
	Vendor       string `json:"vendor"`
	Category     string `json:"category"`
	DetectedName string `json:"detectedName"`
	Version      string `json:"version"`
	Status       string `json:"status"`
}

// SamComplianceHostDetail is the drill-down target for a managed host.
type SamComplianceHostDetail struct {
	Hostname      string                          `json:"hostname"`
	LastSeenAt    *time.Time                      `json:"lastSeenAt"`
	Installations []SamComplianceHostInstallation `json:"installations"`
	Licensed      int64                           `json:"licensed"`
	Expired       int64                           `json:"expired"`
	Unlicensed    int64                           `json:"unlicensed"`
	Banned        int64                           `json:"banned"`
	Inactive      int64                           `json:"inactive"`
}

type SamComplianceRisk struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	ProductID      *int64 `json:"productId,omitempty"`
	ProductName    string `json:"productName,omitempty"`
	Affected       int64  `json:"affected"`
	Cost           int64  `json:"cost"`
	Recommendation string `json:"recommendation"`
}

type samComplianceSummaryOutput struct{ Body SamComplianceSummary }
type samComplianceProductsOutput struct{ Body []SamComplianceProduct }
type samComplianceRisksOutput struct{ Body []SamComplianceRisk }

func Register(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "sam-compliance-summary",
		Method:      http.MethodGet,
		Path:        "/api/sam/compliance/summary",
		Summary:     "KPI комплаенса SAM",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*samComplianceSummaryOutput, error) {
		summary, err := s.Summary(ctx)
		if err != nil {
			return nil, err
		}
		return &samComplianceSummaryOutput{Body: summary}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-compliance-products",
		Method:      http.MethodGet,
		Path:        "/api/sam/compliance/products",
		Summary:     "Баланс куплено/установлено по продуктам",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*samComplianceProductsOutput, error) {
		products, err := s.Products(ctx)
		if err != nil {
			return nil, err
		}
		return &samComplianceProductsOutput{Body: products}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-compliance-risks",
		Method:      http.MethodGet,
		Path:        "/api/sam/compliance/risks",
		Summary:     "Финансовые и юридические риски SAM",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*samComplianceRisksOutput, error) {
		risks, err := s.Risks(ctx)
		if err != nil {
			return nil, err
		}
		return &samComplianceRisksOutput{Body: risks}, nil
	})
}

func summaryFromRow(row db.SamComplianceSummaryRow) SamComplianceSummary {
	summary := SamComplianceSummary{
		Products:               row.Products,
		ActiveProducts:         row.ActiveProducts,
		BannedProducts:         row.BannedProducts,
		Installations:          row.Installations,
		Devices:                row.Devices,
		UnmatchedInstallations: row.UnmatchedInstallations,
		LicensedSeats:          row.LicensedSeats,
		OverusedSeats:          row.OverusedSeats,
		IdleSeats:              row.IdleSeats,
		ExpiredSeats:           row.ExpiredSeats,
		BannedInstallations:    row.BannedInstallations,
		AnnualCost:             row.AnnualCost,
		OveruseCost:            row.OveruseCost,
		IdleCost:               row.IdleCost,
		ExpiredCost:            row.ExpiredCost,
		CostAtRisk:             row.OveruseCost + row.ExpiredCost,
		Licenses:               row.Licenses,
		PurchasedSeats:         row.PurchasedSeats,
	}
	summary.RiskLevel = summaryRiskLevel(summary)
	summary.Recommendations = summaryRecommendations(summary)
	return summary
}

func productFromRow(row db.SamComplianceProductsRow) SamComplianceProduct {
	overused := max(row.InstalledDevices-row.LicensedSeats, 0)
	idle := max(row.LicensedSeats-row.InstalledDevices, 0)
	product := SamComplianceProduct{
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
		OverusedSeats:       overused,
		IdleSeats:           idle,
		AnnualCost:          row.AnnualCost,
		AvgCostPerSeat:      row.AvgCostPerSeat,
	}
	product.DeltaSeats = row.InstalledDevices - row.LicensedSeats
	if product.Status == "banned" {
		product.LicensedSeats = 0
		product.ExpiredSeats = 0
		product.OverusedSeats = 0
		product.IdleSeats = 0
		product.DeltaSeats = product.BannedInstallations
	}
	product.RiskCost = (product.OverusedSeats + product.ExpiredSeats + product.IdleSeats) * row.AvgCostPerSeat
	product.RiskLevel = productRiskLevel(product)
	product.Recommendation = productRecommendation(product)
	return product
}

func summaryRiskLevel(summary SamComplianceSummary) string {
	if summary.OverusedSeats > 0 || summary.ExpiredSeats > 0 || summary.BannedInstallations > 0 {
		return "high"
	}
	if summary.IdleSeats > 0 || summary.UnmatchedInstallations > 0 {
		return "medium"
	}
	return "low"
}

func productRiskLevel(product SamComplianceProduct) string {
	if product.Status == "banned" && product.BannedInstallations > 0 {
		return "high"
	}
	if product.OverusedSeats > 0 || product.ExpiredSeats > 0 {
		return "high"
	}
	if product.IdleSeats > 0 || (product.Status == "outdated" || product.Status == "archived") && product.InstalledDevices > 0 {
		return "medium"
	}
	return "low"
}

func productRecommendation(product SamComplianceProduct) string {
	switch {
	case product.Status == "banned" && product.BannedInstallations > 0:
		return fmt.Sprintf("Удалить запрещённое ПО с %d устройств.", product.BannedInstallations)
	case product.OverusedSeats > 0:
		return fmt.Sprintf("Докупить %d мест или удалить лишние установки.", product.OverusedSeats)
	case product.ExpiredSeats > 0:
		return fmt.Sprintf("Продлить или заменить %d просроченных мест.", product.ExpiredSeats)
	case product.IdleSeats > 0:
		return fmt.Sprintf("Вернуть или перераспределить %d неиспользуемых мест.", product.IdleSeats)
	default:
		return "Действий не требуется."
	}
}

func summaryRecommendations(summary SamComplianceSummary) []string {
	result := make([]string, 0, 5)
	if summary.BannedInstallations > 0 {
		result = append(result, fmt.Sprintf("Удалить запрещённое ПО с %d устройств.", summary.BannedInstallations))
	}
	if summary.OverusedSeats > 0 {
		result = append(result, fmt.Sprintf("Докупить %d мест или удалить лишние установки.", summary.OverusedSeats))
	}
	if summary.ExpiredSeats > 0 {
		result = append(result, fmt.Sprintf("Продлить или заменить %d просроченных мест.", summary.ExpiredSeats))
	}
	if summary.IdleSeats > 0 {
		result = append(result, fmt.Sprintf("Вернуть или перераспределить %d неиспользуемых мест.", summary.IdleSeats))
	}
	if summary.UnmatchedInstallations > 0 {
		result = append(result, fmt.Sprintf("Сопоставить %d установок с каталогом.", summary.UnmatchedInstallations))
	}
	if len(result) == 0 {
		result = append(result, "Критических действий не требуется.")
	}
	return result
}

func buildRisks(summary SamComplianceSummary, products []SamComplianceProduct) []SamComplianceRisk {
	risks := make([]SamComplianceRisk, 0, len(products)+1)
	for _, product := range products {
		id := product.ID
		name := product.Name
		if product.Status == "banned" && product.BannedInstallations > 0 {
			risks = append(risks, SamComplianceRisk{
				Code: "banned_product", Severity: "high", ProductID: &id, ProductName: name,
				Affected: product.BannedInstallations, Recommendation: product.Recommendation,
			})
		}
		if product.OverusedSeats > 0 {
			risks = append(risks, SamComplianceRisk{
				Code: "license_overuse", Severity: "high", ProductID: &id, ProductName: name,
				Affected: product.OverusedSeats, Cost: product.OverusedSeats * product.AvgCostPerSeat,
				Recommendation: product.Recommendation,
			})
		}
		if product.ExpiredSeats > 0 {
			risks = append(risks, SamComplianceRisk{
				Code: "expired_license", Severity: "high", ProductID: &id, ProductName: name,
				Affected: product.ExpiredSeats, Cost: product.ExpiredSeats * product.AvgCostPerSeat,
				Recommendation: product.Recommendation,
			})
		}
		if product.IdleSeats > 0 {
			risks = append(risks, SamComplianceRisk{
				Code: "idle_license", Severity: "medium", ProductID: &id, ProductName: name,
				Affected: product.IdleSeats, Cost: product.IdleSeats * product.AvgCostPerSeat,
				Recommendation: product.Recommendation,
			})
		}
		if (product.Status == "outdated" || product.Status == "archived") && product.InstalledDevices > 0 {
			risks = append(risks, SamComplianceRisk{
				Code: "outdated_product", Severity: "medium", ProductID: &id, ProductName: name,
				Affected: product.InstalledDevices, Recommendation: product.Recommendation,
			})
		}
	}
	if summary.UnmatchedInstallations > 0 {
		risks = append(risks, SamComplianceRisk{
			Code: "unmatched_installations", Severity: "high",
			Affected:       summary.UnmatchedInstallations,
			Recommendation: fmt.Sprintf("Сопоставить %d установок с каталогом.", summary.UnmatchedInstallations),
		})
	}
	sort.SliceStable(risks, func(i, j int) bool {
		if risks[i].Severity != risks[j].Severity {
			return risks[i].Severity == "high"
		}
		if risks[i].Cost != risks[j].Cost {
			return risks[i].Cost > risks[j].Cost
		}
		return risks[i].Affected > risks[j].Affected
	})
	return risks
}
