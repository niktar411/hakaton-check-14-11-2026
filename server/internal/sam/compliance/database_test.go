package compliance

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"hackathon/migrations"
)

func setupComplianceDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	address := os.Getenv("TEST_DATABASE_URL")
	if address == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Path != "/portal_test" {
		t.Fatal("TEST_DATABASE_URL must point to disposable portal_test database")
	}
	migrationDB, err := sql.Open("pgx", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrationDB.Close() })
	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := goose.DownToContext(ctx, migrationDB, ".", 0); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, migrationDB, "."); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	seedComplianceData(t, pool)
	return pool
}

func seedComplianceData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	products := []struct {
		name   string
		status string
	}{
		{"Microsoft Office", "active"},
		{"Microsoft Project", "active"},
		{"Adobe Photoshop", "active"},
		{"Radmin", "banned"},
		{"Slack", "active"},
		{"Java", "active"},
	}
	ids := make(map[string]int64, len(products))
	for _, product := range products {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO sam_products(name,status) VALUES ($1,$2) RETURNING id`, product.name, product.status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[product.name] = id
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_licenses(product_id,license_type,seats,cost_per_seat,currency,expires_at,status) VALUES
		($1,'subscription',100,1000,'RUB',current_date + 30,'active'),
		($2,'subscription',50,500,'RUB',current_date + 30,'active'),
		($3,'subscription',5,0,'RUB',NULL,'active'),
		($4,'subscription',20,200,'RUB',current_date - 1,'active')`,
		ids["Microsoft Office"], ids["Microsoft Project"], ids["Slack"], ids["Java"]); err != nil {
		t.Fatal(err)
	}
	installations := []struct {
		product string
		prefix  string
		count   int
	}{
		{"Microsoft Office", "PC-OFFICE-", 118},
		{"Microsoft Project", "PC-PROJECT-", 10},
		{"Adobe Photoshop", "PC-PHOTO-", 2},
		{"Radmin", "PC-RADMIN-", 8},
		{"Slack", "PC-SLACK-", 5},
	}
	for _, installation := range installations {
		if _, err := pool.Exec(ctx, `INSERT INTO sam_installations(product_id,raw_name,device_name,status)
			SELECT $1, $2, $3 || g, 'installed' FROM generate_series(1,$4) g`,
			ids[installation.product], installation.product, installation.prefix, installation.count); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_installations(raw_name,device_name,status)
		SELECT 'unknown-' || g, 'PC-UNKNOWN-' || g, 'installed' FROM generate_series(1,3) g`); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseComplianceSummaryProductsRisks(t *testing.T) {
	pool := setupComplianceDatabase(t)
	service := NewService(pool)
	ctx := context.Background()

	summary, err := service.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Products != 6 || summary.ActiveProducts != 5 || summary.BannedProducts != 1 {
		t.Fatalf("unexpected product counts: %+v", summary)
	}
	if summary.Installations != 146 || summary.Devices != 146 || summary.UnmatchedInstallations != 3 {
		t.Fatalf("unexpected device counts: %+v", summary)
	}
	if summary.LicensedSeats != 155 || summary.OverusedSeats != 20 || summary.IdleSeats != 40 || summary.ExpiredSeats != 20 {
		t.Fatalf("unexpected license balance: %+v", summary)
	}
	if summary.AnnualCost != 125000 || summary.OveruseCost != 18000 || summary.IdleCost != 20000 || summary.ExpiredCost != 4000 {
		t.Fatalf("unexpected costs: %+v", summary)
	}
	if summary.CostAtRisk != 22000 || summary.BannedInstallations != 8 || summary.RiskLevel != "high" {
		t.Fatalf("unexpected risk KPI: %+v", summary)
	}
	joined := ""
	for _, recommendation := range summary.Recommendations {
		joined += recommendation + " "
	}
	for _, expected := range []string{"запрещённое", "20 мест", "20 просроченных", "40", "3 установок"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("recommendation %q missing in %q", expected, joined)
		}
	}

	products, err := service.Products(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]SamComplianceProduct, len(products))
	for _, product := range products {
		byName[product.Name] = product
	}
	if office := byName["Microsoft Office"]; office.OverusedSeats != 18 || office.RiskCost != 18000 || office.RiskLevel != "high" {
		t.Fatalf("unexpected Office: %+v", office)
	}
	if project := byName["Microsoft Project"]; project.IdleSeats != 40 || project.RiskCost != 20000 || project.RiskLevel != "medium" {
		t.Fatalf("unexpected Project: %+v", project)
	}
	if photoshop := byName["Adobe Photoshop"]; photoshop.OverusedSeats != 2 || photoshop.RiskLevel != "high" {
		t.Fatalf("unexpected Photoshop: %+v", photoshop)
	}
	if radmin := byName["Radmin"]; radmin.BannedInstallations != 8 || radmin.RiskLevel != "high" {
		t.Fatalf("unexpected Radmin: %+v", radmin)
	}
	if slack := byName["Slack"]; slack.RiskLevel != "low" {
		t.Fatalf("unexpected Slack: %+v", slack)
	}
	if java := byName["Java"]; java.ExpiredSeats != 20 || java.RiskLevel != "high" {
		t.Fatalf("unexpected Java: %+v", java)
	}

	risks, err := service.Risks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	codes := make(map[string]bool)
	for _, risk := range risks {
		codes[risk.Code] = true
	}
	for _, code := range []string{"banned_product", "license_overuse", "expired_license", "idle_license", "unmatched_installations"} {
		if !codes[code] {
			t.Fatalf("risk %s missing in %+v", code, risks)
		}
	}
	if risks[0].Severity != "high" {
		t.Fatalf("high risks must come first: %+v", risks)
	}
}
