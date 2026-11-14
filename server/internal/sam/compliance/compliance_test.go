package compliance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
)

type fakeStore struct {
	summary       db.SamComplianceSummaryRow
	products      []db.SamComplianceProductsRow
	product       db.SamComplianceProductRow
	licenses      []db.ListSamProductLicensesRow
	hosts         []db.ListSamProductHostsRow
	host          db.GetSamHostRow
	installations []db.ListSamHostInstallationsRow
	err           error
	productErr    error
	hostErr       error
}

func (f *fakeStore) SamComplianceSummary(context.Context) (db.SamComplianceSummaryRow, error) {
	return f.summary, f.err
}

func (f *fakeStore) SamComplianceProducts(context.Context) ([]db.SamComplianceProductsRow, error) {
	return f.products, f.err
}

func (f *fakeStore) SamComplianceProduct(context.Context, int64) (db.SamComplianceProductRow, error) {
	return f.product, f.productErr
}

func (f *fakeStore) ListSamProductLicenses(context.Context, int64) ([]db.ListSamProductLicensesRow, error) {
	return f.licenses, f.err
}

func (f *fakeStore) ListSamProductHosts(context.Context, int64) ([]db.ListSamProductHostsRow, error) {
	return f.hosts, f.err
}

func (f *fakeStore) GetSamHost(context.Context, int64) (db.GetSamHostRow, error) {
	return f.host, f.hostErr
}

func (f *fakeStore) ListSamHostInstallations(context.Context, int64) ([]db.ListSamHostInstallationsRow, error) {
	return f.installations, f.err
}

func testProducts() []db.SamComplianceProductsRow {
	return []db.SamComplianceProductsRow{
		{ID: 1, Name: "Microsoft Office", Category: "Office", Status: "active", Licenses: 1, PurchasedSeats: 100, LicensedSeats: 100, InstalledDevices: 118, Installations: 120, AvgCostPerSeat: 1000},
		{ID: 2, Name: "Microsoft Project", Category: "Office", Status: "active", Licenses: 1, PurchasedSeats: 50, LicensedSeats: 50, InstalledDevices: 10, Installations: 10, AvgCostPerSeat: 500},
		{ID: 3, Name: "Adobe Photoshop", Category: "Design", Status: "active", InstalledDevices: 2, Installations: 2},
		{ID: 4, Name: "Radmin", Category: "Remote access", Status: "banned", InstalledDevices: 8, Installations: 8},
		{ID: 5, Name: "Slack", Category: "Communication", Status: "active", Licenses: 1, PurchasedSeats: 5, LicensedSeats: 5, InstalledDevices: 5, Installations: 5},
	}
}

func testSummaryRow() db.SamComplianceSummaryRow {
	return db.SamComplianceSummaryRow{
		Products: 5, ActiveProducts: 4, BannedProducts: 1, Assets: 12,
		Installations: 146, Devices: 146, UnmatchedInstallations: 3,
		PurchasedSeats: 155, LicensedSeats: 155, OverusedSeats: 20, IdleSeats: 40,
		AnnualCost: 125000, OveruseCost: 18000, IdleCost: 20000, ExpiredCost: 0,
		BannedInstallations: 8,
	}
}

func TestSummaryMapsKPIAndRecommendations(t *testing.T) {
	service := newService(&fakeStore{summary: testSummaryRow(), products: testProducts()})
	summary, err := service.Summary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.CostAtRisk != 18000 || summary.RiskLevel != "high" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.LicensedSeats != 155 || summary.OverusedSeats != 20 || summary.BannedInstallations != 8 {
		t.Fatalf("summary lost numbers: %+v", summary)
	}
	if summary.CompliancePercent != 40 {
		t.Fatalf("compliance percent: %d", summary.CompliancePercent)
	}
	joined := strings.Join(summary.Recommendations, " ")
	for _, expected := range []string{"запрещённое", "20 мест", "40", "3 установки"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("recommendation %q missing in %q", expected, joined)
		}
	}
	if len(summary.Categories) != 4 || summary.Categories[0].Category != "Office" {
		t.Fatalf("categories: %+v", summary.Categories)
	}
	if len(summary.Statuses) != 4 || summary.Statuses[3].Status != "banned" || summary.Statuses[3].Products != 1 {
		t.Fatalf("statuses: %+v", summary.Statuses)
	}
}

func TestProductsComputeBalanceAndRisk(t *testing.T) {
	service := newService(&fakeStore{products: testProducts()})
	products, err := service.Products(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	office := products[0]
	if office.OverusedSeats != 18 || office.RiskCost != 18000 || office.RiskLevel != "high" || office.DeltaSeats != 18 {
		t.Fatalf("unexpected Office: %+v", office)
	}
	if !strings.Contains(office.Recommendation, "18") {
		t.Fatalf("Office recommendation: %q", office.Recommendation)
	}
	project := products[1]
	if project.IdleSeats != 40 || project.RiskCost != 20000 || project.RiskLevel != "medium" || project.DeltaSeats != -40 {
		t.Fatalf("unexpected Project: %+v", project)
	}
	radmin := products[3]
	if radmin.RiskLevel != "high" || radmin.BannedInstallations != 8 || radmin.OverusedSeats != 0 || !strings.Contains(radmin.Recommendation, "запрещённое") {
		t.Fatalf("unexpected Radmin: %+v", radmin)
	}
	slack := products[4]
	if slack.RiskLevel != "low" || slack.Recommendation != "Действий не требуется." {
		t.Fatalf("unexpected Slack: %+v", slack)
	}
}

func TestRisksIncludeBannedOveruseIdleAndUnmatched(t *testing.T) {
	service := newService(&fakeStore{summary: testSummaryRow(), products: testProducts()})
	risks, err := service.Risks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	codes := make(map[string]bool)
	for _, risk := range risks {
		codes[risk.Code] = true
	}
	for _, code := range []string{"banned_product", "license_overuse", "idle_license", "unmatched_installations"} {
		if !codes[code] {
			t.Fatalf("risk %s missing in %+v", code, risks)
		}
	}
	if risks[0].Severity != "high" {
		t.Fatalf("high risks must come first: %+v", risks)
	}
}

func TestProductDetailMapsHostLicenseStatus(t *testing.T) {
	service := newService(&fakeStore{
		product: db.SamComplianceProductRow{ID: 4, Name: "Radmin", Status: "banned", InstalledDevices: 2},
		hosts: []db.ListSamProductHostsRow{
			{AssetID: 7, Hostname: "alpha", ProductStatus: "banned", HasActiveLicense: true},
			{AssetID: 8, Hostname: "beta", ProductStatus: "active"},
		},
	})
	detail, err := service.Product(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Hosts[0].LicenseStatus != "banned" || detail.Hosts[1].LicenseStatus != "unlicensed" {
		t.Fatalf("hosts: %+v", detail.Hosts)
	}
}

func TestProductNotFoundWrapsSentinel(t *testing.T) {
	service := newService(&fakeStore{productErr: pgx.ErrNoRows})
	_, err := service.Product(context.Background(), 404)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestHostDetailCountsInstallationStates(t *testing.T) {
	productID := int64(1)
	service := newService(&fakeStore{
		host: db.GetSamHostRow{ID: 2, Hostname: "beta", Installations: 4},
		installations: []db.ListSamHostInstallationsRow{
			{ID: 1, ProductID: pgtype.Int8{Int64: productID, Valid: true}, DetectedName: "Office", ProductName: pgtype.Text{String: "Microsoft Office", Valid: true}, ProductStatus: pgtype.Text{String: "active", Valid: true}, HasActiveLicense: true},
			{ID: 2, ProductID: pgtype.Int8{Int64: productID, Valid: true}, DetectedName: "Radmin", ProductName: pgtype.Text{String: "Radmin", Valid: true}, ProductStatus: pgtype.Text{String: "banned", Valid: true}},
			{ID: 3, ProductID: pgtype.Int8{Int64: productID, Valid: true}, DetectedName: "Legacy", ProductName: pgtype.Text{String: "Legacy", Valid: true}, ProductStatus: pgtype.Text{String: "active", Valid: true}, HasExpiredLicense: true},
			{ID: 4, DetectedName: "Unknown tool"},
		},
	})
	detail, err := service.Host(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Licensed != 1 || detail.Banned != 1 || detail.Expired != 1 || detail.Unlicensed != 1 {
		t.Fatalf("unexpected counts: %+v", detail)
	}
	if detail.Installations[3].Product != "Unknown tool" || detail.Installations[3].ProductID != nil {
		t.Fatalf("unmatched installation: %+v", detail.Installations[3])
	}
}
