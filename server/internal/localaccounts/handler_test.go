package localaccounts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/identity"
)

func newHTTPTestServer(store Store) http.Handler {
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.Header.Get("X-Test-IP")
			ctx := identity.WithRequestIP(r.Context(), ip)
			if r.Header.Get("X-Test-Principal") != "" {
				username := r.Header.Get("X-Test-Principal")
				role := "member"
				id := int64(2)
				if username == "root" {
					role = "admin"
					id = 1
				}
				principal := identity.Principal{
					Session:  db.Session{Username: username},
					Account:  db.LocalAccount{ID: id, Username: username, Role: role, Enabled: true},
					AuthKind: "local", IP: ip,
				}
				ctx = identity.WithPrincipal(ctx, principal)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	config := huma.DefaultConfig("local accounts test", "1.0.0")
	config.DocsPath = ""
	config.OpenAPIPath = ""
	config.SchemasPath = ""
	api := humachi.New(router, config)
	service := newUnitService(store)
	RegisterPublic(api, service)
	RegisterAdmin(api, service)
	return router
}

func apiRequest(t *testing.T, handler http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func adminHeaders() map[string]string {
	return map[string]string{"X-Test-Principal": "root", "X-Test-IP": "10.77.0.2"}
}

func TestLocalLoginHTTPResponse(t *testing.T) {
	store := newFakeStore(testDBAccount(1, "root", "admin", validTestHash(t)))
	handler := newHTTPTestServer(store)
	response := apiRequest(t, handler, http.MethodPost, "/api/auth/local-login", LoginRequest{
		Username: "root", Password: "correct-horse-battery", Audience: "admin",
	}, map[string]string{"Origin": "https://portal.example", "X-Test-IP": "10.77.0.2"})
	if response.Code != http.StatusOK {
		t.Fatalf("login status %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("login response is cacheable")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookieName || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("unexpected cookies: %#v", cookies)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["csrf"] == "" || body["account"] == nil {
		t.Fatalf("incomplete login response: %s", response.Body.String())
	}
	lowerBody := strings.ToLower(response.Body.String())
	if strings.Contains(lowerBody, "password") || strings.Contains(lowerBody, "session") ||
		strings.Contains(response.Body.String(), cookies[0].Value) || strings.Contains(response.Body.String(), validTestHash(t)) {
		t.Fatalf("login response leaked a secret: %s", response.Body.String())
	}
}

func TestLocalLoginHTTPValidation(t *testing.T) {
	store := newFakeStore(testDBAccount(1, "root", "admin", validTestHash(t)))
	handler := newHTTPTestServer(store)
	tests := []struct {
		name    string
		request LoginRequest
		origin  string
		status  int
	}{
		{name: "origin", request: LoginRequest{Username: "root", Password: "correct-horse-battery", Audience: "coding"}, origin: "https://evil.example", status: http.StatusForbidden},
		{name: "audience", request: LoginRequest{Username: "root", Password: "correct-horse-battery", Audience: "other"}, origin: "https://portal.example", status: http.StatusUnprocessableEntity},
		{name: "username", request: LoginRequest{Username: "ROOT", Password: "correct-horse-battery", Audience: "coding"}, origin: "https://portal.example", status: http.StatusUnprocessableEntity},
		{name: "password", request: LoginRequest{Username: "root", Password: "wrong-password", Audience: "coding"}, origin: "https://portal.example", status: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := apiRequest(t, handler, http.MethodPost, "/api/auth/local-login", test.request, map[string]string{
				"Origin": test.origin, "X-Test-IP": "192.0.2.10",
			})
			if response.Code != test.status {
				t.Fatalf("status %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
			if len(response.Result().Cookies()) != 0 {
				t.Fatal("failed login set a cookie")
			}
		})
	}
}

func TestAccountHTTPAdministration(t *testing.T) {
	admin := testDBAccount(1, "root", "admin", "root-secret-hash")
	member := testDBAccount(2, "member", "member", "member-secret-hash")
	store := newFakeStore(admin, member)
	store.conversations[5] = db.Conversation{ID: 5, Username: "member", Phase: "ready", CreatedAt: pgtype.Timestamptz{Time: testNow, Valid: true}}
	store.messages[5] = []db.Message{{ID: 1, ConversationID: 5, Role: "user", Content: "hello"}}
	handler := newHTTPTestServer(store)

	response := apiRequest(t, handler, http.MethodGet, "/api/admin/accounts", nil, adminHeaders())
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "secret-hash") || strings.Contains(response.Body.String(), "passwordHash") {
		t.Fatalf("unsafe account list: %d %s", response.Code, response.Body.String())
	}

	create := validCreateRequest()
	response = apiRequest(t, handler, http.MethodPost, "/api/admin/accounts", create, adminHeaders())
	if response.Code != http.StatusOK {
		t.Fatalf("create status %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), create.Password) || strings.Contains(response.Body.String(), "passwordHash") {
		t.Fatal("create response leaked password material")
	}

	response = apiRequest(t, handler, http.MethodPost, "/api/admin/accounts/2/profile", ProfileRequest{
		DisplayName: "Updated", Color: "#aBb123", PersonalContext: "updated", Tone: "calm",
	}, adminHeaders())
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Updated") {
		t.Fatalf("profile status %d: %s", response.Code, response.Body.String())
	}

	response = apiRequest(t, handler, http.MethodPost, "/api/admin/accounts/1/enabled", map[string]bool{"enabled": false}, adminHeaders())
	if response.Code != http.StatusConflict {
		t.Fatalf("self-disable status %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodPost, "/api/admin/accounts/2/enabled", map[string]bool{"enabled": false}, adminHeaders())
	if response.Code != http.StatusOK {
		t.Fatalf("disable status %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodPost, "/api/admin/accounts/2/sessions/reset", nil, adminHeaders())
	if response.Code != http.StatusOK {
		t.Fatalf("reset status %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodPost, "/api/admin/accounts/2/password", PasswordRequest{Password: "replacement-password"}, adminHeaders())
	if response.Code != http.StatusOK || !verifyPassword(store.accounts[2].PasswordHash, "replacement-password") {
		t.Fatalf("password status %d: %s", response.Code, response.Body.String())
	}

	response = apiRequest(t, handler, http.MethodGet, "/api/admin/accounts/2/conversations", nil, adminHeaders())
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":5`) {
		t.Fatalf("conversations status %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodGet, "/api/admin/conversations/5", nil, adminHeaders())
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "hello") {
		t.Fatalf("conversation status %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminHTTPAuthorizationAndErrors(t *testing.T) {
	store := newFakeStore(testDBAccount(1, "root", "admin", ""))
	handler := newHTTPTestServer(store)
	for _, test := range []struct {
		name    string
		headers map[string]string
		status  int
	}{
		{name: "unauthenticated", headers: map[string]string{"X-Test-IP": "10.77.0.2"}, status: http.StatusUnauthorized},
		{name: "member", headers: map[string]string{"X-Test-Principal": "member", "X-Test-IP": "10.77.0.2"}, status: http.StatusForbidden},
		{name: "wrong IP", headers: map[string]string{"X-Test-Principal": "root", "X-Test-IP": "10.77.0.3"}, status: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := apiRequest(t, handler, http.MethodGet, "/api/admin/accounts", nil, test.headers)
			if response.Code != test.status {
				t.Fatalf("status %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
		})
	}

	response := apiRequest(t, handler, http.MethodPost, "/api/admin/accounts/999/profile", ProfileRequest{
		DisplayName: "Missing", Color: "#112233", PersonalContext: "", Tone: "",
	}, adminHeaders())
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing account status %d: %s", response.Code, response.Body.String())
	}
	invalid := validCreateRequest()
	invalid.Role = "owner"
	response = apiRequest(t, handler, http.MethodPost, "/api/admin/accounts", invalid, adminHeaders())
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid create status %d: %s", response.Code, response.Body.String())
	}
	response = apiRequest(t, handler, http.MethodGet, "/api/admin/accounts/0/conversations", nil, adminHeaders())
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid id status %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminHTTPStorageFailure(t *testing.T) {
	store := newFakeStore()
	store.errors["list"] = assertStorageError{}
	handler := newHTTPTestServer(store)
	response := apiRequest(t, handler, http.MethodGet, "/api/admin/accounts", nil, adminHeaders())
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "storage.unavailable") {
		t.Fatalf("storage status %d: %s", response.Code, response.Body.String())
	}
}

type assertStorageError struct{}

func (assertStorageError) Error() string { return "storage failed" }
