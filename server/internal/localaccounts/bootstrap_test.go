package localaccounts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
)

type fakeBootstrapDatabase struct {
	query string
	args  []any
	row   pgx.Row
	calls int
}

func (f *fakeBootstrapDatabase) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	f.calls++
	f.query = query
	f.args = append([]any(nil), args...)
	return f.row
}

type fakeBootstrapRow struct {
	account db.LocalAccount
	err     error
}

func (r fakeBootstrapRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 11 {
		return errors.New("unexpected bootstrap scan destination count")
	}
	*(dest[0].(*int64)) = r.account.ID
	*(dest[1].(*string)) = r.account.Username
	*(dest[2].(*string)) = r.account.DisplayName
	*(dest[3].(*string)) = r.account.PasswordHash
	*(dest[4].(*string)) = r.account.Role
	*(dest[5].(*bool)) = r.account.Enabled
	*(dest[6].(*string)) = r.account.Color
	*(dest[7].(*string)) = r.account.PersonalContext
	*(dest[8].(*string)) = r.account.Tone
	*(dest[9].(*pgtype.Timestamptz)) = r.account.CreatedAt
	*(dest[10].(*pgtype.Timestamptz)) = r.account.UpdatedAt
	return nil
}

func TestBootstrapAdminUpsert(t *testing.T) {
	stored := testDBAccount(7, "root", "admin", "")
	database := &fakeBootstrapDatabase{row: fakeBootstrapRow{account: stored}}
	account, err := BootstrapAdmin(context.Background(), database, "root", "bootstrap-password", "#Aa0102")
	if err != nil {
		t.Fatal(err)
	}
	if account.ID != 7 || account.Username != "root" || account.Role != "admin" {
		t.Fatalf("unexpected bootstrap result: %#v", account)
	}
	if database.calls != 1 || len(database.args) != 3 {
		t.Fatalf("unexpected bootstrap query: calls=%d args=%d", database.calls, len(database.args))
	}
	if database.args[0] != "root" || database.args[2] != "#Aa0102" {
		t.Fatalf("unexpected bootstrap arguments: %#v", database.args)
	}
	hash, ok := database.args[1].(string)
	if !ok || hash == "bootstrap-password" || !verifyPassword(hash, "bootstrap-password") {
		t.Fatal("bootstrap did not persist an Argon2id password hash")
	}
	for _, fragment := range []string{"ON CONFLICT(username) DO UPDATE", "role = 'admin'", "enabled = true"} {
		if !strings.Contains(database.query, fragment) {
			t.Fatalf("bootstrap query does not contain %q", fragment)
		}
	}
}

func TestBootstrapAdminValidationAndErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		username string
		password string
		color    string
	}{
		{name: "username", username: "ROOT", password: "bootstrap-password", color: "#112233"},
		{name: "password", username: "root", password: "short", color: "#112233"},
		{name: "color", username: "root", password: "bootstrap-password", color: "blue"},
	} {
		t.Run(test.name, func(t *testing.T) {
			database := &fakeBootstrapDatabase{}
			if _, err := BootstrapAdmin(context.Background(), database, test.username, test.password, test.color); !errors.Is(err, ErrValidation) {
				t.Fatalf("got %v", err)
			}
			if database.calls != 0 {
				t.Fatal("invalid input reached the database")
			}
		})
	}

	databaseErr := errors.New("database failed")
	database := &fakeBootstrapDatabase{row: fakeBootstrapRow{err: databaseErr}}
	if _, err := BootstrapAdmin(context.Background(), database, "root", "bootstrap-password", "#112233"); !errors.Is(err, databaseErr) {
		t.Fatalf("database error not preserved: %v", err)
	}
}
