package sam

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
)

type fakeStore struct {
	dashboard db.SamDashboardRow
	products  []db.SamPublicProductsRow
	licenses  []db.ListSamLicensesRow
	assets    []db.ListSamAssetsRow
	findings  []db.ListSamFindingsRow
	matches        []db.ListSamMatchesRow
	err            error
	nameExists     bool
	productByIDErr error
}

func (s *fakeStore) SamDashboard(context.Context) (db.SamDashboardRow, error) {
	return s.dashboard, s.err
}

func (s *fakeStore) SamPublicProducts(context.Context, db.SamPublicProductsParams) ([]db.SamPublicProductsRow, error) {
	return s.products, s.err
}

func (s *fakeStore) ListSamLicenses(context.Context, db.ListSamLicensesParams) ([]db.ListSamLicensesRow, error) {
	return s.licenses, s.err
}

func (s *fakeStore) ListSamAssets(context.Context, db.ListSamAssetsParams) ([]db.ListSamAssetsRow, error) {
	return s.assets, s.err
}

func (s *fakeStore) ListSamFindings(context.Context, db.ListSamFindingsParams) ([]db.ListSamFindingsRow, error) {
	return s.findings, s.err
}

func (s *fakeStore) ListSamMatches(context.Context, db.ListSamMatchesParams) ([]db.ListSamMatchesRow, error) {
	return s.matches, s.err
}

// Edit stubs: фиксированное поведение для тестов контракта и прав.

func (s *fakeStore) SamProductNameExists(context.Context, string) (bool, error) {
	return s.nameExists, nil
}

func (s *fakeStore) SamProductByID(context.Context, int64) (db.SamProduct, error) {
	return db.SamProduct{}, nil
}

func (s *fakeStore) SamDeleteProduct(context.Context, int64) (int64, error) {
	return 1, nil
}

func (s *fakeStore) SamCreateAliases(context.Context, db.SamCreateAliasesParams) ([]db.SamProductAlias, error) {
	return nil, nil
}

func (s *fakeStore) SamDeleteAliases(context.Context, []int64) (int64, error) {
	return 1, nil
}

func (s *fakeStore) SamEditProductByID(context.Context, int64) (db.SamEditProductByIDRow, error) {
	return db.SamEditProductByIDRow{}, s.productByIDErr
}

func (s *fakeStore) SamEditCreateProduct(_ context.Context, arg db.SamEditCreateProductParams) (db.SamEditCreateProductRow, error) {
	return db.SamEditCreateProductRow{
		ID:   1,
		Name: arg.Name,
	}, nil
}

func (s *fakeStore) SamEditUpdateProduct(context.Context, db.SamEditUpdateProductParams) (db.SamEditUpdateProductRow, error) {
	return db.SamEditUpdateProductRow{}, nil
}

func (s *fakeStore) SamEditProductUsage(context.Context, int64) (db.SamEditProductUsageRow, error) {
	return db.SamEditProductUsageRow{}, nil
}

func (s *fakeStore) SamEditLicenseByID(context.Context, int64) (db.SamEditLicenseByIDRow, error) {
	return db.SamEditLicenseByIDRow{}, nil
}

func (s *fakeStore) SamEditCreateLicense(context.Context, db.SamEditCreateLicenseParams) (db.SamEditCreateLicenseRow, error) {
	return db.SamEditCreateLicenseRow{}, nil
}

func (s *fakeStore) SamEditUpdateLicense(context.Context, db.SamEditUpdateLicenseParams) (db.SamEditUpdateLicenseRow, error) {
	return db.SamEditUpdateLicenseRow{}, nil
}

func (s *fakeStore) SamEditLicenseView(context.Context, int64) (db.SamEditLicenseViewRow, error) {
	return db.SamEditLicenseViewRow{}, nil
}

func (s *fakeStore) SamEditDeleteLicense(context.Context, int64) (int64, error) {
	return 1, nil
}

func (s *fakeStore) SamEditInstallationByID(context.Context, int64) (db.SamEditInstallationByIDRow, error) {
	return db.SamEditInstallationByIDRow{}, nil
}

func (s *fakeStore) SamEditCreateInstallation(context.Context, db.SamEditCreateInstallationParams) (db.SamEditCreateInstallationRow, error) {
	return db.SamEditCreateInstallationRow{}, nil
}

func (s *fakeStore) SamEditUpdateInstallation(context.Context, db.SamEditUpdateInstallationParams) (db.SamEditUpdateInstallationRow, error) {
	return db.SamEditUpdateInstallationRow{}, nil
}

func (s *fakeStore) SamEditInstallationView(context.Context, int64) (db.SamEditInstallationViewRow, error) {
	return db.SamEditInstallationViewRow{}, nil
}

func (s *fakeStore) SamEditDeleteInstallation(context.Context, int64) (int64, error) {
	return 1, nil
}

func (s *fakeStore) SamEditAliasesByProduct(context.Context, int64) ([]db.SamEditAliasesByProductRow, error) {
	return nil, nil
}

func (s *fakeStore) SamEditAuditInsert(context.Context, db.SamEditAuditInsertParams) error {
	return nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func TestDashboardMapsAggregates(t *testing.T) {
	store := &fakeStore{dashboard: db.SamDashboardRow{
		Products: 4, Assets: 5, Licenses: 6, SeatsTotal: 70, SeatsUsed: 42,
		OpenFindings: 3, PendingMatches: 2, ExpiringLicenses: 1, UnlicensedInstallations: 9,
	}}
	got, err := newService(store).Dashboard(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := Overview{
		Products: 4, Assets: 5, Licenses: 6, SeatsTotal: 70, SeatsUsed: 42,
		OpenFindings: 3, PendingMatches: 2, ExpiringLicenses: 1, UnlicensedInstallations: 9,
	}
	if got != want {
		t.Fatalf("dashboard = %#v, want %#v", got, want)
	}
}

func TestProductsMapUsageCounts(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{products: []db.SamPublicProductsRow{{
		ID: 7, Name: "Design Suite", Vendor: "Acme", Category: "Design",
		SectionCode: "04", ClassCode: "04.12", LicenseMetric: "user",
		RegistryRf: true, RegistryEntry: "12345", IsTrackable: false,
		LicenseCount: 2, InstallationCount: 5, UpdatedAt: timestamp(now),
	}}}
	got, err := newService(store).Products(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("products = %d, want 1", len(got))
	}
	product := got[0]
	if product.ID != 7 || product.Name != "Design Suite" || product.Vendor != "Acme" ||
		product.Category != "Design" || product.LicenseCount != 2 || product.InstallationCount != 5 ||
		!product.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected product: %#v", product)
	}
	if product.SectionCode != "04" || product.ClassCode != "04.12" || product.LicenseMetric != "user" ||
		!product.RegistryRf || product.RegistryEntry != "12345" || product.IsTrackable {
		t.Fatalf("classification and registry fields were not mapped: %#v", product)
	}
}

func TestLicensesMapStatusAndOptionalDate(t *testing.T) {
	store := &fakeStore{licenses: []db.ListSamLicensesRow{
		{
			ID: 1, ProductName: "Design Suite", ProductVendor: "Acme", Kind: "subscription",
			SeatsTotal: 10, SeatsUsed: 4, Contract: "Договор-1",
			ValidUntil: pgtype.Date{Time: time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC), Valid: true},
			Status: "active",
		},
		{
			ID: 2, ProductName: "Editor", ProductVendor: "Acme", Kind: "perpetual",
			SeatsTotal: 3, SeatsUsed: 3, Status: "active",
		},
	}}
	got, err := newService(store).Licenses(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("licenses = %d, want 2", len(got))
	}
	if got[0].ValidUntil == nil || *got[0].ValidUntil != "2027-01-15" || got[0].Status != "active" ||
		got[0].Contract != "Договор-1" {
		t.Fatalf("unexpected dated license: %#v", got[0])
	}
	if got[1].ValidUntil != nil || got[1].Status != "active" {
		t.Fatalf("unexpected perpetual license: %#v", got[1])
	}
}

func TestAssetsMapOptionalLastSeen(t *testing.T) {
	seen := time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)
	store := &fakeStore{assets: []db.ListSamAssetsRow{
		{ID: 1, Hostname: "alpha", Platform: "Linux", Owner: "team", SoftwareCount: 8, LastSeenAt: timestamp(seen)},
		{ID: 2, Hostname: "beta", Platform: "macOS", SoftwareCount: 0},
	}}
	got, err := newService(store).Assets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got[0].LastSeenAt == nil || !got[0].LastSeenAt.Equal(seen) {
		t.Fatalf("unexpected last seen: %#v", got[0])
	}
	if got[1].LastSeenAt != nil {
		t.Fatalf("missing last seen became %v", got[1].LastSeenAt)
	}
}

func TestFindingsAndMatchesMapNames(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{
		findings: []db.ListSamFindingsRow{{
			ID: 3, Code: "unlicensed", Severity: "high", Status: "open",
			Details: "No active license", ProductName: "Design Suite", AssetHostname: "alpha",
			DetectedAt: timestamp(now),
		}},
		matches: []db.ListSamMatchesRow{{
			ID: 4, DetectedName: "design-suite", Version: "12.1", AssetHostname: "alpha",
			ProductName: "Design Suite", Confidence: 0.92, Status: "suggested", CreatedAt: timestamp(now),
		}},
	}
	service := newService(store)
	findings, err := service.Findings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if findings[0].Product != "Design Suite" || findings[0].Asset != "alpha" || findings[0].Severity != "high" {
		t.Fatalf("unexpected finding: %#v", findings[0])
	}
	matches, err := service.Matches(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if matches[0].Product != "Design Suite" || matches[0].Asset != "alpha" ||
		matches[0].Confidence != 0.92 || matches[0].Status != "suggested" {
		t.Fatalf("unexpected match: %#v", matches[0])
	}
}

func TestAssistantDerivesOnlyNonZeroSuggestions(t *testing.T) {
	store := &fakeStore{dashboard: db.SamDashboardRow{
		ExpiringLicenses: 0, OpenFindings: 2, PendingMatches: 0, UnlicensedInstallations: 1,
	}}
	got, err := newService(store).Assistant(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Suggestions) != 2 {
		t.Fatalf("suggestions = %#v, want 2", got.Suggestions)
	}
	if got.Suggestions[0] != (Suggestion{Kind: "open_findings", Count: 2, Path: "/sam/compliance"}) {
		t.Fatalf("unexpected first suggestion: %#v", got.Suggestions[0])
	}
	if got.Suggestions[1] != (Suggestion{Kind: "unlicensed_installations", Count: 1, Path: "/sam/matching"}) {
		t.Fatalf("unexpected second suggestion: %#v", got.Suggestions[1])
	}
}

func TestAssistantReturnsEmptySlice(t *testing.T) {
	got, err := newService(&fakeStore{}).Assistant(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.Suggestions == nil || len(got.Suggestions) != 0 {
		t.Fatalf("suggestions = %#v, want empty non-nil slice", got.Suggestions)
	}
}

func TestStorageErrorsAreWrapped(t *testing.T) {
	store := &fakeStore{err: errors.New("database down")}
	_, err := newService(store).Dashboard(t.Context())
	if err == nil || !strings.Contains(err.Error(), "load SAM dashboard: database down") {
		t.Fatalf("unexpected error: %v", err)
	}
}
