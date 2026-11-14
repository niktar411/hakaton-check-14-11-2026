package assistant

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"hackathon/internal/sam/guard"
	"hackathon/migrations"
)

func setupAssistantDatabase(t *testing.T) *pgxpool.Pool {
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
	seedAssistantData(t, pool)
	return pool
}

func seedAssistantData(t *testing.T, pool *pgxpool.Pool) {
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
		VALUES ($1,'subscription',10,1000,'RUB',current_date + 30,'active'), ($2,'subscription',5,0,'RUB',current_date - 1,'active'), ($3,'perpetual',3,500,'RUB',NULL,'active')`, chrome, slack, notepad); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_installations(product_id,raw_name,device_name,status)
		SELECT $1, 'Google Chrome', 'pc-' || g, 'installed' FROM generate_series(1,8) g`, chrome); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_installations(product_id,raw_name,device_name,status)
		SELECT $1, 'Slack', 'pc-' || g, 'installed' FROM generate_series(1,2) g`, slack); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sam_installations(product_id,raw_name,device_name,status) VALUES
		(NULL,'chromium','pc-11','installed'),(NULL,'chromium','pc-12','installed'),(NULL,'chromium','pc-13','installed'),
		(NULL,'libreoffice','pc-14','installed'),(NULL,'libreoffice','pc-15','installed'),
		(NULL,'unknown-tool','pc-16','installed')`); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseFallbackAnswersWithExactNumbers(t *testing.T) {
	pool := setupAssistantDatabase(t)
	service := NewService(pool, &fakeModel{ready: false}, guard.NewLimiter(2, 1))
	questions := []struct {
		prompt string
		want   []string
	}{
		{"Дай общую сводку SAM", []string{"продуктов — 3", "установок — 16", "несопоставленных установок — 6", "лицензий — 3", "18 мест", "истёкших лицензий — 1"}},
		{"Сколько установок всего?", []string{"Обезличенные установки (все продукты): 16"}},
		{"Сколько мест осталось у Google Chrome?", []string{"оплачено 10 мест, установлено 8, свободно 2"}},
		{"Сводка лицензий", []string{"3 лицензий", "18 мест", "1 истёкших", "11500 RUB/год"}},
		{"Какие риски?", []string{"Установки без сопоставленного продукта: 6 строк", "Активные лицензии с истёкшим сроком: 1 строк"}},
		{"Покажи несопоставленные имена", []string{"chromium — 3 установок на 3 устройствах", "Всего несопоставленных установок: 6"}},
		{"Найди Chrome", []string{"Найдено продуктов по запросу «chrome»: 1"}},
		{"Список активных продуктов", []string{"Продуктов со статусом active: 3"}},
		{"Покажи все продукты", []string{"Продуктов: 3"}},
		{"Где установлен Slack?", []string{"Обезличенные установки по продукту «Slack»: 2"}},
	}
	for index, question := range questions {
		answer, err := service.Chat(context.Background(), "niktar411", 0, question.prompt, fmt.Sprintf("%016d", index+1))
		if err != nil {
			t.Fatalf("%s: %v", question.prompt, err)
		}
		if !answer.Fallback {
			t.Fatalf("%s: expected fallback, got model path", question.prompt)
		}
		for _, expected := range question.want {
			if !strings.Contains(answer.Answer, expected) {
				t.Fatalf("%s: expected %q in %q", question.prompt, expected, answer.Answer)
			}
		}
	}
	conversations, err := service.Conversations(context.Background(), "niktar411", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != len(questions) {
		t.Fatalf("expected %d conversations, got %d", len(questions), len(conversations))
	}
	history, err := service.Conversation(context.Background(), "niktar411", conversations[len(conversations)-1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Messages) != 2 {
		t.Fatalf("expected user and assistant messages, got %d", len(history.Messages))
	}
}
