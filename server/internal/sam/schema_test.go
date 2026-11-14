package sam

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"hackathon/migrations"
)

func schemaTestDB(t *testing.T) (*pgxpool.Pool, *sql.DB) {
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
	if err := goose.DownToContext(t.Context(), migrationDB, ".", 0); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, migrationDB
}

func columnExists(t *testing.T, pool *pgxpool.Pool, table, column string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)`,
		table, column).Scan(&exists)
	if err != nil {
		t.Fatal(err)
	}
	return exists
}

func tableExists(t *testing.T, pool *pgxpool.Pool, table string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
		table).Scan(&exists)
	if err != nil {
		t.Fatal(err)
	}
	return exists
}

func TestSchemaContract(t *testing.T) {
	pool, migrationDB := schemaTestDB(t)
	if err := goose.UpToContext(t.Context(), migrationDB, ".", 10); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"sam_vendors", "sam_employees", "sam_devices", "sam_imports", "sam_audit_log"} {
		if !tableExists(t, pool, table) {
			t.Fatalf("table %s is missing", table)
		}
	}
	for _, column := range []struct{ table, name string }{
		{"sam_products", "vendor_id"},
		{"sam_products", "section_code"},
		{"sam_products", "class_code"},
		{"sam_products", "license_metric"},
		{"sam_products", "registry_rf"},
		{"sam_products", "registry_entry"},
		{"sam_products", "is_trackable"},
		{"sam_licenses", "contract"},
		{"sam_installations", "device_id"},
		{"sam_installations", "employee_id"},
	} {
		if !columnExists(t, pool, column.table, column.name) {
			t.Fatalf("column %s.%s is missing", column.table, column.name)
		}
	}

	ctx := t.Context()
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO sam_products(name, status) VALUES ('Legacy', 'outdated') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("outdated status must be accepted: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_products(name, status) VALUES ('Broken', 'inactive')`); err == nil {
		t.Fatal("inactive status must be rejected by the schema")
	}
	var metric string
	err := pool.QueryRow(ctx, `INSERT INTO sam_products(name, license_metric, registry_rf, is_trackable, section_code, class_code)
		VALUES ('Tracked', 'user', true, false, '04', '05.12') RETURNING license_metric`).Scan(&metric)
	if err != nil {
		t.Fatalf("extended product columns must accept values: %v", err)
	}
	if metric != "user" {
		t.Fatalf("license_metric = %q, want user", metric)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_products(name, license_metric) VALUES ('Bad', 'pet')`); err == nil {
		t.Fatal("unknown license metric must be rejected by the schema")
	}
}

func TestReferenceDataMigration(t *testing.T) {
	pool, migrationDB := schemaTestDB(t)
	ctx := t.Context()
	if err := goose.UpToContext(ctx, migrationDB, ".", 8); err != nil {
		t.Fatal(err)
	}
	var productID, installationID int64
	if err := pool.QueryRow(ctx, `INSERT INTO sam_products(name, vendor, status) VALUES ('Office', '  Microsoft  ', 'inactive') RETURNING id`).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO sam_installations(product_id, raw_name, device_name, user_name)
		VALUES ($1, 'WINWORD.EXE', 'PC-0001', 'Иван') RETURNING id`, productID).Scan(&installationID); err != nil {
		t.Fatal(err)
	}

	if err := goose.UpToContext(ctx, migrationDB, ".", 10); err != nil {
		t.Fatal(err)
	}

	var vendorName string
	if err := pool.QueryRow(ctx, `SELECT v.name FROM sam_products p JOIN sam_vendors v ON v.id = p.vendor_id WHERE p.id = $1`, productID).Scan(&vendorName); err != nil {
		t.Fatalf("vendor link was not populated: %v", err)
	}
	if vendorName != "Microsoft" {
		t.Fatalf("vendor name = %q, want Microsoft", vendorName)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM sam_products WHERE id = $1`, productID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "outdated" {
		t.Fatalf("status = %q, want outdated", status)
	}
	var deviceCode, employeeCode string
	err := pool.QueryRow(ctx, `SELECT d.device_code, e.employee_code
		FROM sam_installations i
		JOIN sam_devices d ON d.id = i.device_id
		JOIN sam_employees e ON e.id = i.employee_id
		WHERE i.id = $1`, installationID).Scan(&deviceCode, &employeeCode)
	if err != nil {
		t.Fatalf("installation links were not populated: %v", err)
	}
	if deviceCode != "PC-0001" {
		t.Fatalf("device_code = %q, want PC-0001", deviceCode)
	}
	if len(employeeCode) < 4 || employeeCode[:4] != "EMP-" {
		t.Fatalf("employee_code = %q, want EMP- prefix", employeeCode)
	}
	var owner sql.NullInt64
	if err := pool.QueryRow(ctx, `SELECT employee_id FROM sam_devices WHERE device_code = 'PC-0001'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if !owner.Valid {
		t.Fatal("device owner was not derived from installations")
	}

	if err := goose.DownToContext(ctx, migrationDB, ".", 8); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM sam_products WHERE id = $1`, productID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "inactive" {
		t.Fatalf("rollback status = %q, want inactive", status)
	}
}
