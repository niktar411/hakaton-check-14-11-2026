package operations

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	db "hackathon/internal/db/sqlc"
	"hackathon/migrations"
)

func TestDatabaseAuthorizationAndMigrations(t *testing.T) {
	address := os.Getenv("TEST_DATABASE_URL")
	if address == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping disposable portal_test database test")
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Path != "/portal_test" {
		t.Fatal("TEST_DATABASE_URL must point to disposable portal_test database")
	}
	ctx := t.Context()
	migrationDB, err := sql.Open("pgx", address)
	if err != nil {
		t.Fatal(err)
	}
	defer migrationDB.Close()
	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
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
	defer pool.Close()
	queries := db.New(pool)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		username := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if err := json.NewEncoder(w).Encode(map[string]string{"username": username, "state": "active"}); err != nil {
			t.Error(err)
		}
	}))
	defer provider.Close()
	config := Config{GitLabURL: provider.URL, PublicURL: "http://portal", AdminUser: "niktar411", AdminIP: "10.77.0.2", StaticDir: t.TempDir()}
	service := NewService(pool, config)
	router := service.Router()
	for _, username := range []string{"niktar411", "hensy"} {
		err := queries.CreateSession(ctx, db.CreateSessionParams{ID: digest(username), Username: username, AccessToken: pgtype.Text{String: username, Valid: true}, CsrfToken: "csrf-test", ExpiresAt: timestamp(time.Now().Add(time.Hour))})
		if err != nil {
			t.Fatal(err)
		}
	}
	adminAccount, err := queries.CreateLocalAccount(ctx, db.CreateLocalAccountParams{Username: "niktar411", DisplayName: "Administrator", PasswordHash: "unused-in-this-test", Role: "admin", Color: "#2a78ff"})
	if err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateSession(ctx, db.CreateSessionParams{ID: digest("local-admin"), Username: adminAccount.Username, CsrfToken: "csrf-test", ExpiresAt: timestamp(time.Now().Add(time.Hour)), AccountID: pgtype.Int8{Int64: adminAccount.ID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		user, ip, method, path, origin, csrf string
		code                                 int
	}{
		{"niktar411", "10.77.0.3", "GET", "/api/admin/dashboard", "", "", 403},
		{"hensy", "10.77.0.2", "GET", "/api/admin/dashboard", "", "", 403},
		{"niktar411", "10.77.0.2", "POST", "/api/admin/peers/egor", "http://evil", "csrf-test", 403},
		{"niktar411", "10.77.0.2", "POST", "/api/admin/peers/egor", "http://portal", "wrong", 403},
		{"niktar411", "10.77.0.2", "GET", "/api/me", "", "", 200},
		{"local-admin", "10.77.0.2", "GET", "/api/admin/dashboard", "", "", 200},
	} {
		req := httptest.NewRequest(tc.method, "http://portal"+tc.path, strings.NewReader(`{"action":"disable"}`))
		req.RemoteAddr = tc.ip + ":12345"
		req.AddCookie(&http.Cookie{Name: "portal_session", Value: tc.user})
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("X-CSRF-Token", tc.csrf)
		req.Header.Set("X-Forwarded-For", "10.77.0.2")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != tc.code {
			t.Errorf("%s %s %s: got %d want %d", tc.user, tc.ip, tc.path, response.Code, tc.code)
		}
	}
	conversation, err := queries.CreateConversation(ctx, "niktar411")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetConversation(ctx, db.GetConversationParams{ID: conversation.ID, Username: "hensy"}); err == nil {
		t.Fatal("conversation exposed to another user")
	}
	if err := goose.DownToContext(ctx, migrationDB, ".", 0); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, migrationDB, "."); err != nil {
		t.Fatal(err)
	}
}
