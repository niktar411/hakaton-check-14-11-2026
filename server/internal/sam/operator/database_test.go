package operator

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"hackathon/internal/sam/guard"
	"hackathon/migrations"
)

func setupOperatorDatabase(t *testing.T) *pgxpool.Pool {
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
	seedOperatorData(t, pool)
	return pool
}

func seedOperatorData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var chrome, slack, notepad int64
	if err := pool.QueryRow(ctx, `INSERT INTO sam_products(name,vendor,version,category,status) VALUES ('Google Chrome','Google','120','Браузеры','active') RETURNING id`).Scan(&chrome); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO sam_products(name,vendor,version,category,status) VALUES ('Slack','Salesforce','','Мессенджеры','active') RETURNING id`).Scan(&slack); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO sam_products(name,vendor,version,category,status) VALUES ('Notepad++','Don Ho','8.6','Редакторы','active') RETURNING id`).Scan(&notepad); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_licenses(product_id,license_type,seats,cost_per_seat,currency,expires_at,status)
		VALUES ($1,'subscription',10,1000,'RUB',current_date + 30,'active'), ($2,'perpetual',3,0,'RUB',NULL,'active')`, chrome, notepad); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_installations(product_id,raw_name,device_name,status) VALUES
		($1,'Google Chrome','pc-1','installed'),($1,'Google Chrome','pc-2','installed'),
		(NULL,'chromium','pc-11','installed'),(NULL,'chromium','pc-12','installed'),(NULL,'chromium','pc-13','installed'),
		(NULL,'chrome.exe','pc-21','installed'),(NULL,'chrome.exe','pc-22','installed'),
		(NULL,'libreoffice','pc-31','installed')`, chrome); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var value int64
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestDatabaseOperatorLifecycle(t *testing.T) {
	pool := setupOperatorDatabase(t)
	service := NewService(pool, &fakeModel{ready: false}, guard.NewLimiter(2, 1), Config{})
	ctx := context.Background()

	action, err := service.Propose(ctx, 0, "niktar411", "match_aliases", `{"product":"Google Chrome","all":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if action.Affected != 7 {
		t.Fatalf("expected 5 installations and 2 aliases, got %d", action.Affected)
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_product_aliases`) != 0 {
		t.Fatal("preview wrote aliases")
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_installations WHERE product_id IS NULL`) != 6 {
		t.Fatal("preview changed installations")
	}
	if _, err := service.Apply(ctx, "niktar411", true, action.ID, "wrong"); err == nil {
		t.Fatal("expected the confirmation phrase to be required")
	} else {
		requireStatus(t, err, 422)
	}
	applied, err := service.Apply(ctx, "niktar411", true, action.ID, allProductsPhrase)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != "applied" || applied.Affected != 7 {
		t.Fatalf("unexpected applied action: %+v", applied)
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_product_aliases WHERE product_id=(SELECT id FROM sam_products WHERE name='Google Chrome')`) != 2 {
		t.Fatal("aliases were not created")
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_installations WHERE product_id IS NULL`) != 1 {
		t.Fatal("installations were not mapped")
	}
	if _, err := service.Apply(ctx, "niktar411", true, action.ID, allProductsPhrase); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_product_aliases`) != 2 {
		t.Fatal("repeated apply created duplicate aliases")
	}
	if countRows(t, pool, `SELECT count(*) FROM audit_events WHERE action='sam_operator.apply' AND status='completed'`) != 1 {
		t.Fatal("apply audit record is missing")
	}
	undone, err := service.Undo(ctx, "niktar411", action.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undone.Status != "cancelled" || undone.UndoneAt == nil {
		t.Fatalf("unexpected undo result: %+v", undone)
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_product_aliases`) != 0 {
		t.Fatal("undo did not delete aliases")
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_installations WHERE product_id IS NULL`) != 6 {
		t.Fatal("undo did not clear installation mapping")
	}
	if countRows(t, pool, `SELECT count(*) FROM audit_events WHERE action='sam_operator.undo' AND status='completed'`) != 1 {
		t.Fatal("undo audit record is missing")
	}

	stale, err := service.Propose(ctx, 0, "niktar411", "set_status", `{"query":"Notepad++","status":"outdated"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sam_products SET status='outdated' WHERE name='Notepad++'`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, "niktar411", true, stale.ID, ""); err == nil {
		t.Fatal("expected stale preview rejection")
	} else {
		requireStatus(t, err, 409)
	}
	var staleStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM sam_ai_actions WHERE id=$1`, stale.ID).Scan(&staleStatus); err != nil {
		t.Fatal(err)
	}
	if staleStatus != "proposed" {
		t.Fatalf("stale apply must leave the action proposed, got %s", staleStatus)
	}

	created, err := service.Propose(ctx, 0, "niktar411", "create_product", `{"name":"Visual Studio Code","vendor":"Microsoft","status":"active","aliases":["vscode","code.exe"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if created.Affected != 3 {
		t.Fatalf("unexpected create affected rows: %d", created.Affected)
	}
	if _, err := service.Apply(ctx, "niktar411", true, created.ID, ""); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_products WHERE name='Visual Studio Code'`) != 1 {
		t.Fatal("product was not created")
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_product_aliases WHERE product_id=(SELECT id FROM sam_products WHERE name='Visual Studio Code')`) != 2 {
		t.Fatal("product aliases were not created")
	}
	if _, err := service.Undo(ctx, "niktar411", created.ID); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_products WHERE name='Visual Studio Code'`) != 0 {
		t.Fatal("undo did not delete the created product")
	}

	license, err := service.Propose(ctx, 0, "niktar411", "update_license", `{"product":"Google Chrome","seats":25,"expiresAt":"2027-12-31"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, "niktar411", true, license.ID, ""); err != nil {
		t.Fatal(err)
	}
	var seats int64
	var expiry string
	if err := pool.QueryRow(ctx, `SELECT seats, expires_at::text FROM sam_licenses WHERE product_id=(SELECT id FROM sam_products WHERE name='Google Chrome')`).Scan(&seats, &expiry); err != nil {
		t.Fatal(err)
	}
	if seats != 25 || expiry != "2027-12-31" {
		t.Fatalf("license was not updated: seats=%d expiry=%s", seats, expiry)
	}
	if _, err := service.Undo(ctx, "niktar411", license.ID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT seats FROM sam_licenses WHERE product_id=(SELECT id FROM sam_products WHERE name='Google Chrome')`).Scan(&seats); err != nil {
		t.Fatal(err)
	}
	if seats != 10 {
		t.Fatalf("undo did not restore license seats: %d", seats)
	}

	category, err := service.Propose(ctx, 0, "niktar411", "set_category", `{"query":"Google Chrome","category":"Браузеры и расширения"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, "niktar411", true, category.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Undo(ctx, "niktar411", category.ID); err != nil {
		t.Fatal(err)
	}
	var restoredCategory string
	if err := pool.QueryRow(ctx, `SELECT category FROM sam_products WHERE name='Google Chrome'`).Scan(&restoredCategory); err != nil {
		t.Fatal(err)
	}
	if restoredCategory != "Браузеры" {
		t.Fatalf("undo did not restore category: %s", restoredCategory)
	}

	archived, err := service.Propose(ctx, 0, "niktar411", "archive_product", `{"query":"Slack"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, "niktar411", true, archived.ID, ""); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_products WHERE name='Slack' AND status='archived'`) != 1 {
		t.Fatal("archive was not applied")
	}
	if _, err := service.Undo(ctx, "niktar411", archived.ID); err != nil {
		t.Fatal(err)
	}
	if countRows(t, pool, `SELECT count(*) FROM sam_products WHERE name='Slack' AND status='active'`) != 1 {
		t.Fatal("undo did not restore archived product")
	}

	listed, err := service.Actions(ctx, "niktar411", false, 0, "applied", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected no applied actions after undo, got %d", len(listed))
	}
}

func TestDatabaseApplyRequiresAdmin(t *testing.T) {
	pool := setupOperatorDatabase(t)
	service := NewService(pool, &fakeModel{ready: false}, guard.NewLimiter(2, 1), Config{})
	ctx := context.Background()
	action, err := service.Propose(ctx, 0, "member", "archive_product", `{"query":"Slack"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(ctx, "member", false, action.ID, ""); err == nil {
		t.Fatal("expected admin requirement")
	} else {
		requireStatus(t, err, 403)
	}
}
