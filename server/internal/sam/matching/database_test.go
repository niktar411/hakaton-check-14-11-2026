package matching

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"hackathon/migrations"
)

func setupMatchingDatabase(t *testing.T) *pgxpool.Pool {
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
	seedMatchingData(t, pool)
	return pool
}

func seedMatchingData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, name := range []string{"Google Chrome", "Notepad++", "Microsoft Office", "Adobe Photoshop", "Slack"} {
		if _, err := pool.Exec(ctx, `INSERT INTO sam_products(name,status) VALUES ($1,'active')`, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"chrome.exe", "Google_Chrome_v2", "chrome_setup", "Хром экзе", "Google Chrome (x64)", "unknown-zzz"} {
		if _, err := pool.Exec(ctx, `INSERT INTO sam_installations(raw_name,device_name,status) VALUES ($1,'PC-0001','installed')`, raw); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDatabaseMatchingRunConfirmReject(t *testing.T) {
	pool := setupMatchingDatabase(t)
	service := NewService(pool)
	ctx := context.Background()

	result, err := service.Run(ctx, "niktar411", 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 6 || result.Matched != 5 || result.Skipped != 1 || result.Candidates != 5 {
		t.Fatalf("unexpected run stats: %+v", result)
	}
	if count(t, pool, `SELECT count(*) FROM sam_installations WHERE product_id IS NOT NULL`) != 0 {
		t.Fatal("run mapped installations before confirmation")
	}
	if count(t, pool, `SELECT count(*) FROM sam_product_aliases`) != 0 {
		t.Fatal("run created aliases before confirmation")
	}

	pending, err := service.Candidates(ctx, "pending", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var chromeCandidate, rejectCandidate int64
	for _, candidate := range pending {
		switch candidate.RawName {
		case "chrome.exe":
			chromeCandidate = candidate.ID
		case "Google_Chrome_v2":
			rejectCandidate = candidate.ID
		}
	}
	if chromeCandidate == 0 || rejectCandidate == 0 {
		t.Fatalf("expected chrome candidates, got %+v", pending)
	}

	confirmed, err := service.Confirm(ctx, "niktar411", chromeCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "confirmed" || confirmed.ProductName != "Google Chrome" {
		t.Fatalf("unexpected confirmed candidate: %+v", confirmed)
	}
	if count(t, pool, `SELECT count(*) FROM sam_product_aliases WHERE normalized_alias='chrome'`) != 1 {
		t.Fatal("alias was not created")
	}
	if count(t, pool, `SELECT count(*) FROM sam_installations WHERE product_id IS NOT NULL`) != 1 {
		t.Fatal("installation was not mapped")
	}
	if _, err := service.Confirm(ctx, "niktar411", chromeCandidate); err == nil {
		t.Fatal("repeated confirm must fail")
	} else {
		requireStatus(t, err, 409)
	}

	rejected, err := service.Reject(ctx, "niktar411", rejectCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "rejected" {
		t.Fatalf("unexpected rejected candidate: %+v", rejected)
	}

	summary, err := service.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Unmatched != 5 || summary.Pending != 3 || summary.Confirmed != 1 || summary.Rejected != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if count(t, pool, `SELECT count(*) FROM audit_events WHERE action IN ('sam_matching.confirm','sam_matching.reject') AND status='completed'`) != 2 {
		t.Fatal("matching audit records are missing")
	}
}

func count(t *testing.T, pool *pgxpool.Pool, query string) int64 {
	t.Helper()
	var value int64
	if err := pool.QueryRow(context.Background(), query).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
