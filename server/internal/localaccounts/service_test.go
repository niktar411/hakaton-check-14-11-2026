package localaccounts

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/identity"
)

var testNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func testConfig() Config {
	return Config{
		PublicURL: "https://portal.example", PlannerURL: "https://planner.example",
		AdminUser: "root", AdminIP: "10.77.0.2", SecureCookies: true,
	}
}

func testDBAccount(id int64, username, role, passwordHash string) db.LocalAccount {
	return db.LocalAccount{
		ID: id, Username: username, DisplayName: username + " display", PasswordHash: passwordHash,
		Role: role, Enabled: true, Color: "#1122aA", PersonalContext: "context", Tone: "direct",
		CreatedAt: pgtype.Timestamptz{Time: testNow.Add(-time.Hour), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: testNow.Add(-time.Hour), Valid: true},
	}
}

func newUnitService(store Store) *Service {
	service := newService(store, testConfig())
	service.now = func() time.Time { return testNow }
	return service
}

func administratorContext(config Config) context.Context {
	account := testDBAccount(1, config.AdminUser, "admin", "")
	principal := identity.Principal{
		Session: db.Session{Username: config.AdminUser}, Account: account,
		AuthKind: "local", IP: config.AdminIP,
	}
	return identity.WithPrincipal(context.Background(), principal)
}

func loginContext(ip string) context.Context {
	return identity.WithRequestIP(context.Background(), ip)
}

func validCreateRequest() CreateAccountRequest {
	return CreateAccountRequest{
		Username: "newmember", DisplayName: "New Member", Password: "a-secure-password",
		Role: "member", Color: "#Aa1200", PersonalContext: "profile", Tone: "concise",
	}
}

func TestLoginCreatesOpaqueSession(t *testing.T) {
	store := newFakeStore(testDBAccount(1, "root", "admin", validTestHash(t)))
	service := newUnitService(store)
	result, cookie, err := service.Login(loginContext("10.77.0.2"), LoginRequest{
		Username: "root", Password: "correct-horse-battery", Audience: "admin",
	}, "https://portal.example")
	if err != nil {
		t.Fatal(err)
	}
	if result.CSRF == "" || cookie.Value == "" || result.CSRF == cookie.Value {
		t.Fatal("session and CSRF tokens must be independent non-empty values")
	}
	if cookie.Name != SessionCookieName || cookie.Path != "/" || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != int(sessionLifetime.Seconds()) || !cookie.Expires.Equal(testNow.Add(sessionLifetime)) {
		t.Fatalf("unsafe or incorrect session cookie: %#v", cookie)
	}
	stored, ok := store.sessions[sessionDigest(cookie.Value)]
	if !ok {
		t.Fatal("session was not stored under the SHA-256 digest")
	}
	if stored.ID == cookie.Value || stored.AccessToken.Valid || stored.CsrfToken != result.CSRF ||
		stored.Username != "root" || !stored.AccountID.Valid || stored.AccountID.Int64 != 1 {
		t.Fatalf("incorrect stored session: %#v", stored)
	}
	if !stored.ExpiresAt.Time.Equal(testNow.Add(sessionLifetime)) {
		t.Fatalf("session expires at %s", stored.ExpiresAt.Time)
	}
	if result.Account.Username != "root" || result.Account.Role != "admin" {
		t.Fatalf("incorrect login account: %#v", result.Account)
	}
}

func TestLoginOriginAndAdministratorRestrictions(t *testing.T) {
	tests := []struct {
		name     string
		username string
		audience string
		origin   string
		ip       string
		disabled bool
		want     error
	}{
		{name: "missing origin", username: "root", audience: "admin", ip: "10.77.0.2", want: ErrInvalidOrigin},
		{name: "untrusted origin", username: "root", audience: "admin", origin: "https://evil.example", ip: "10.77.0.2", want: ErrInvalidOrigin},
		{name: "planner coding login", username: "member", audience: "coding", origin: "https://planner.example", ip: "192.0.2.4"},
		{name: "member cannot enter admin", username: "member", audience: "admin", origin: "https://portal.example", ip: "10.77.0.2", want: ErrAdminAccess},
		{name: "wrong admin IP", username: "root", audience: "admin", origin: "https://portal.example", ip: "10.77.0.3", want: ErrAdminAccess},
		{name: "missing request IP", username: "root", audience: "admin", origin: "https://portal.example", want: ErrAdminAccess},
		{name: "disabled", username: "root", audience: "admin", origin: "https://portal.example", ip: "10.77.0.2", disabled: true, want: ErrInvalidCredentials},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			admin := testDBAccount(1, "root", "admin", validTestHash(t))
			member := testDBAccount(2, "member", "member", validTestHash(t))
			if test.username == "root" {
				admin.Enabled = !test.disabled
			}
			store := newFakeStore(admin, member)
			service := newUnitService(store)
			_, cookie, err := service.Login(loginContext(test.ip), LoginRequest{
				Username: test.username, Password: "correct-horse-battery", Audience: test.audience,
			}, test.origin)
			if !errors.Is(err, test.want) {
				t.Fatalf("got error %v, want %v", err, test.want)
			}
			if test.want == nil && (err != nil || cookie == nil) {
				t.Fatal("successful login did not return a cookie")
			}
			if test.want != nil && cookie != nil {
				t.Fatal("failed login returned a session cookie")
			}
		})
	}
}

func TestAdministratorLoginAcceptsEveryConfiguredAddress(t *testing.T) {
	admin := testDBAccount(1, "root", "admin", validTestHash(t))
	store := newFakeStore(admin)
	config := testConfig()
	config.AdminIP = "10.77.0.2, 10.77.0.6"
	service := newService(store, config)
	service.now = func() time.Time { return testNow }
	for _, ip := range []string{"10.77.0.2", "10.77.0.6"} {
		_, cookie, err := service.Login(loginContext(ip), LoginRequest{
			Username: "root", Password: "correct-horse-battery", Audience: "admin",
		}, "https://portal.example")
		if err != nil || cookie == nil {
			t.Fatalf("address %s rejected: %v", ip, err)
		}
	}
}

func TestLoginRejectsUnknownAccountAndStorageFailure(t *testing.T) {	service := newUnitService(newFakeStore())
	_, _, err := service.Login(loginContext("192.0.2.1"), LoginRequest{
		Username: "nobody", Password: "incorrect-password", Audience: "coding",
	}, "https://portal.example")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown account returned %v", err)
	}

	storageErr := errors.New("database unavailable")
	store := newFakeStore()
	store.errors["get_by_username"] = storageErr
	service = newUnitService(store)
	_, _, err = service.Login(loginContext("192.0.2.1"), LoginRequest{
		Username: "nobody", Password: "incorrect-password", Audience: "coding",
	}, "https://portal.example")
	if !errors.Is(err, ErrStorage) || !errors.Is(err, storageErr) {
		t.Fatalf("storage failure was not preserved: %v", err)
	}
}

func TestLoginThrottlingIsPerIPAndUsername(t *testing.T) {
	store := newFakeStore(testDBAccount(1, "root", "admin", validTestHash(t)))
	service := newUnitService(store)
	now := testNow
	service.now = func() time.Time { return now }
	request := LoginRequest{Username: "root", Password: "wrong-password", Audience: "coding"}
	for attempt := 1; attempt <= maxLoginFailures; attempt++ {
		_, _, err := service.Login(loginContext("192.0.2.10"), request, "https://portal.example")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d returned %v", attempt, err)
		}
	}
	_, _, err := service.Login(loginContext("192.0.2.10"), request, "https://portal.example")
	if !errors.Is(err, ErrLoginThrottled) {
		t.Fatalf("blocked attempt returned %v", err)
	}

	request.Password = "correct-horse-battery"
	if _, _, err := service.Login(loginContext("192.0.2.11"), request, "https://portal.example"); err != nil {
		t.Fatalf("another IP was incorrectly blocked: %v", err)
	}
	now = now.Add(loginLockDuration + time.Second)
	if _, _, err := service.Login(loginContext("192.0.2.10"), request, "https://portal.example"); err != nil {
		t.Fatalf("lock did not expire: %v", err)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestLoginRandomSourceFailureDoesNotCreateSession(t *testing.T) {
	store := newFakeStore(testDBAccount(1, "root", "admin", validTestHash(t)))
	service := newUnitService(store)
	randomErr := errors.New("random source failed")
	service.random = errorReader{err: randomErr}
	_, _, err := service.Login(loginContext("10.77.0.2"), LoginRequest{
		Username: "root", Password: "correct-horse-battery", Audience: "coding",
	}, "https://portal.example")
	if !errors.Is(err, randomErr) || len(store.sessions) != 0 {
		t.Fatalf("random failure: err=%v sessions=%d", err, len(store.sessions))
	}
}

func TestCreateListAndUpdateProfile(t *testing.T) {
	store := newFakeStore(testDBAccount(1, "root", "admin", "secret-hash"))
	service := newUnitService(store)
	ctx := administratorContext(testConfig())
	request := validCreateRequest()
	request.DisplayName = "  New Member  "
	created, err := service.CreateAccount(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.DisplayName != "New Member" || created.Role != "member" || !created.Enabled {
		t.Fatalf("incorrect created account: %#v", created)
	}
	stored := store.accounts[created.ID]
	if stored.PasswordHash == request.Password || !verifyPassword(stored.PasswordHash, request.Password) {
		t.Fatal("created account did not store a verifiable password hash")
	}
	if _, err := service.CreateAccount(ctx, request); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate username returned %v", err)
	}

	accounts, err := service.ListAccounts(ctx, 0, 0)
	if err != nil || len(accounts) != 2 {
		t.Fatalf("list accounts: %v, %#v", err, accounts)
	}
	profile, err := service.UpdateProfile(ctx, created.ID, ProfileRequest{
		DisplayName: "  Updated Name ", Color: "#0102fF", PersonalContext: "new context", Tone: "friendly",
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName != "Updated Name" || profile.Color != "#0102fF" || profile.PersonalContext != "new context" {
		t.Fatalf("profile not updated: %#v", profile)
	}
}

func TestCreateAccountValidation(t *testing.T) {
	service := newUnitService(newFakeStore())
	ctx := administratorContext(testConfig())
	tests := map[string]func(*CreateAccountRequest){
		"username":         func(r *CreateAccountRequest) { r.Username = "INVALID" },
		"display name":     func(r *CreateAccountRequest) { r.DisplayName = " \t" },
		"password":         func(r *CreateAccountRequest) { r.Password = "short" },
		"role":             func(r *CreateAccountRequest) { r.Role = "owner" },
		"color":            func(r *CreateAccountRequest) { r.Color = "red" },
		"personal context": func(r *CreateAccountRequest) { r.PersonalContext = string(make([]byte, 16001)) },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			request := validCreateRequest()
			change(&request)
			if _, err := service.CreateAccount(ctx, request); !errors.Is(err, ErrValidation) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestEveryAdminMethodRequiresAdminBeforeStorage(t *testing.T) {
	store := newFakeStore()
	service := newUnitService(store)
	ctx := context.Background()
	tests := map[string]func() error{
		"list":   func() error { _, err := service.ListAccounts(ctx, 100, 0); return err },
		"create": func() error { _, err := service.CreateAccount(ctx, validCreateRequest()); return err },
		"profile": func() error {
			_, err := service.UpdateProfile(ctx, 2, ProfileRequest{DisplayName: "Name", Color: "#112233"})
			return err
		},
		"enabled":       func() error { _, err := service.SetEnabled(ctx, 2, false); return err },
		"reset":         func() error { _, err := service.ResetSessions(ctx, 2); return err },
		"password":      func() error { _, err := service.SetPassword(ctx, 2, "a-secure-password"); return err },
		"conversations": func() error { _, err := service.AccountConversations(ctx, 2, 100, 0); return err },
		"conversation":  func() error { _, err := service.ConversationForAdmin(ctx, 2); return err },
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("unauthenticated call succeeded")
			}
		})
	}
	if len(store.calls) != 0 {
		t.Fatalf("unauthenticated calls reached storage: %v", store.calls)
	}
}

func TestDisableAndResetSessions(t *testing.T) {
	admin := testDBAccount(1, "root", "admin", "")
	member := testDBAccount(2, "member", "member", "")
	store := newFakeStore(admin, member)
	store.sessions["member-session"] = db.CreateSessionParams{ID: "member-session", AccountID: pgtype.Int8{Int64: 2, Valid: true}}
	service := newUnitService(store)
	ctx := administratorContext(testConfig())

	if _, err := service.SetEnabled(ctx, 1, false); !errors.Is(err, ErrCannotDisableAdmin) {
		t.Fatalf("disable current admin returned %v", err)
	}
	account, err := service.SetEnabled(ctx, 2, false)
	if err != nil || account.Enabled {
		t.Fatalf("disable member: account=%#v err=%v", account, err)
	}
	if len(store.sessions) != 0 {
		t.Fatal("disabling did not revoke sessions")
	}
	if _, err := service.SetEnabled(ctx, 999, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account returned %v", err)
	}

	store.sessions["new-session"] = db.CreateSessionParams{ID: "new-session", AccountID: pgtype.Int8{Int64: 2, Valid: true}}
	if _, err := service.ResetSessions(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if len(store.sessions) != 0 {
		t.Fatal("session reset left sessions behind")
	}
}

func TestSetPasswordHashesAndRevokesSessions(t *testing.T) {
	member := testDBAccount(2, "member", "member", validTestHash(t))
	store := newFakeStore(testDBAccount(1, "root", "admin", ""), member)
	store.sessions["session"] = db.CreateSessionParams{ID: "session", AccountID: pgtype.Int8{Int64: 2, Valid: true}}
	service := newUnitService(store)
	ctx := administratorContext(testConfig())
	if _, err := service.SetPassword(ctx, 2, "replacement-password"); err != nil {
		t.Fatal(err)
	}
	stored := store.accounts[2]
	if stored.PasswordHash == validTestHash(t) || !verifyPassword(stored.PasswordHash, "replacement-password") {
		t.Fatal("replacement password was not hashed and stored")
	}
	if len(store.sessions) != 0 {
		t.Fatal("password change did not revoke sessions")
	}
	if _, err := service.SetPassword(ctx, 999, "replacement-password"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account returned %v", err)
	}
}

func TestConversationAdministration(t *testing.T) {
	member := testDBAccount(2, "member", "member", "")
	store := newFakeStore(testDBAccount(1, "root", "admin", ""), member)
	store.conversations[4] = db.Conversation{ID: 4, Username: "member", Phase: "ready", CreatedAt: pgtype.Timestamptz{Time: testNow, Valid: true}}
	store.conversations[7] = db.Conversation{ID: 7, Username: "member", Phase: "clarifying", CreatedAt: pgtype.Timestamptz{Time: testNow, Valid: true}}
	store.conversations[9] = db.Conversation{ID: 9, Username: "other", Phase: "ready"}
	store.messages[4] = []db.Message{
		{ID: 8, ConversationID: 4, Role: "assistant", Content: "answer"},
		{ID: 3, ConversationID: 4, Role: "user", Content: "question"},
	}
	service := newUnitService(store)
	ctx := administratorContext(testConfig())
	conversations, err := service.AccountConversations(ctx, 2, 100, 0)
	if err != nil || len(conversations) != 2 || conversations[0].ID != 7 || conversations[1].ID != 4 {
		t.Fatalf("conversations: %#v, %v", conversations, err)
	}
	detail, err := service.ConversationForAdmin(ctx, 4)
	if err != nil || len(detail.Messages) != 2 || detail.Messages[0].ID != 3 || detail.Messages[1].Content != "answer" {
		t.Fatalf("conversation detail: %#v, %v", detail, err)
	}
	if _, err := service.ConversationForAdmin(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing conversation returned %v", err)
	}
}

func TestStorageAndPaginationErrors(t *testing.T) {
	store := newFakeStore()
	storageErr := errors.New("query failed")
	store.errors["list"] = storageErr
	service := newUnitService(store)
	ctx := administratorContext(testConfig())
	if _, err := service.ListAccounts(ctx, 100, 0); !errors.Is(err, ErrStorage) || !errors.Is(err, storageErr) {
		t.Fatalf("list storage error: %v", err)
	}
	for _, page := range [][2]int32{{-1, 0}, {101, 0}, {10, -1}} {
		if _, err := service.ListAccounts(ctx, page[0], page[1]); !errors.Is(err, ErrValidation) {
			t.Fatalf("page %v returned %v", page, err)
		}
	}
}
