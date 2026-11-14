package compliance

import (
	"context"
	"strings"
	"testing"

	db "hackathon/internal/db/sqlc"
)

type fakeStore struct {
	summary  db.SamComplianceSummaryRow
	products []db.SamComplianceProductsRow
	err      error
}

func (f *fakeStore) SamComplianceSummary(context.Context) (db.SamComplianceSummaryRow, error) {
	return f.summary, f.err
}

func (f *fakeStore) SamComplianceProducts(context.Context) ([]db.SamComplianceProductsRow, error) {
	return f.products, f.err
}

func (f *fakeStore) SamComplianceProduct(context.Context, int64) (db.SamComplianceProductRow, error) {
	return db.SamComplianceProductRow{}, f.err
}

func (f *fakeStore) ListSamProductLicenses(context.Context, int64) ([]db.ListSamProductLicensesRow, error) {
	return nil, f.err
}

func (f *fakeStore) ListSamProductHosts(context.Context, int64) ([]db.ListSamProductHostsRow, error) {
	return nil, f.err
}

func (f *fakeStore) GetSamHost(context.Context, string) (db.GetSamHostRow, error) {
	return db.GetSamHostRow{}, f.err
}

func (f *fakeStore) ListSamHostInstallations(context.Context, string) ([]db.ListSamHostInstallationsRow, error) {
	return nil, f.err
}

func testRows() []db.SamComplianceProductsRow {
	return []db.SamComplianceProductsRow{
		{ID: 1, Name: "Microsoft Office", Status: "active", LicensedSeats: 100, InstalledDevices: 118, Installations: 118, AvgCostPerSeat: 1000},
		{ID: 2, Name: "Microsoft Project", Status: "active", LicensedSeats: 50, InstalledDevices: 10, Installations: 10, AvgCostPerSeat: 500},
		{ID: 3, Name: "Adobe Photoshop", Status: "active", InstalledDevices: 2, Installations: 2},
		{ID: 4, Name: "Radmin", Status: "banned", InstalledDevices: 8, Installations: 8, BannedInstallations: 8},
		{ID: 5, Name: "Slack", Status: "active", LicensedSeats: 5, InstalledDevices: 5, Installations: 5},
	}
}

func testSummaryRow() db.SamComplianceSummaryRow {
	return db.SamComplianceSummaryRow{
		Products: 5, ActiveProducts: 4, BannedProducts: 1,
		Installations: 146, Devices: 146, UnmatchedInstallations: 3,
		LicensedSeats: 155, OverusedSeats: 20, IdleSeats: 40, ExpiredSeats: 0,
		AnnualCost: 125000, OveruseCost: 18000, IdleCost: 20000, ExpiredCost: 0,
		BannedInstallations: 8,
	}
}

func TestSummaryMapsKPIAndRecommendations(t *testing.T) {
	service := newService(&fakeStore{summary: testSummaryRow(), products: testRows()})
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
	joined := strings.Join(summary.Recommendations, " ")
	for _, expected := range []string{"запрещённое", "20 мест", "40", "3 установок"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("recommendation %q missing in %q", expected, joined)
		}
	}
}

func TestProductsComputeBalanceAndRisk(t *testing.T) {
	service := newService(&fakeStore{summary: testSummaryRow(), products: testRows()})
	products, err := service.Products(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	office := products[0]
	if office.OverusedSeats != 18 || office.RiskCost != 18000 || office.RiskLevel != "high" {
		t.Fatalf("unexpected Office: %+v", office)
	}
	if !strings.Contains(office.Recommendation, "18") {
		t.Fatalf("Office recommendation: %q", office.Recommendation)
	}
	project := products[1]
	if project.IdleSeats != 40 || project.RiskCost != 20000 || project.RiskLevel != "medium" {
		t.Fatalf("unexpected Project: %+v", project)
	}
	radmin := products[3]
	if radmin.RiskLevel != "high" || !strings.Contains(radmin.Recommendation, "запрещённое") {
		t.Fatalf("unexpected Radmin: %+v", radmin)
	}
	slack := products[4]
	if slack.RiskLevel != "low" || slack.Recommendation != "Действий не требуется." {
		t.Fatalf("unexpected Slack: %+v", slack)
	}
}

func TestRisksIncludeBannedOveruseIdleAndUnmatched(t *testing.T) {
	service := newService(&fakeStore{summary: testSummaryRow(), products: testRows()})
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
