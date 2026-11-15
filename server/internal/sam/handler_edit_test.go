package sam

import (
	"context"
	"io"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/identity"
)

// editTestAPI registers the editing surface behind a middleware that injects
// the given principal, mirroring the production chi group.
func editTestAPI(t *testing.T, principal *identity.Principal, store *fakeStore) *httptest.Server {
	t.Helper()
	svc := &Service{store: store, editRun: func(ctx context.Context, fn func(Store) error) error {
		return fn(store)
	}}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			if principal != nil {
				ctx = identity.WithPrincipal(ctx, *principal)
			}
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	api := humachi.New(r, testConfig())
	RegisterEdit(api, svc)
	return httptest.NewServer(r)
}

func testConfig() huma.Config {
	config := huma.DefaultConfig("SAM edit test", "1.0.0")
	config.OpenAPIPath = ""
	config.SchemasPath = ""
	return config
}

func principal(username string) *identity.Principal {
	return &identity.Principal{
		AuthKind: "local",
		Account:  db.LocalAccount{Username: username, Enabled: true},
		Session:  db.Session{Username: username},
	}
}

func readBody(response *http.Response) string {
	data, _ := io.ReadAll(response.Body)
	return string(data)
}

func TestCreateProductRequiresAuth(t *testing.T) {
	server := editTestAPI(t, nil, &fakeStore{})
	defer server.Close()
	response, err := http.Post(server.URL+"/api/sam/products", "application/json",
		strings.NewReader(`{"name":"Git","status":"active","licenseMetric":"device"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body %s", response.StatusCode, readBody(response))
	}
}

func TestCreateProductWritesAudit(t *testing.T) {
	store := &fakeStore{}
	server := editTestAPI(t, principal("egor"), store)
	defer server.Close()
	response, err := http.Post(server.URL+"/api/sam/products", "application/json",
		strings.NewReader(`{"name":"Git","vendor":"Git","status":"active","licenseMetric":"device","isTrackable":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var body Product
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "Git" || body.ID != 1 {
		t.Fatalf("unexpected product: %+v", body)
	}
}

func TestCreateProductConflict(t *testing.T) {
	store := &fakeStore{nameExists: true}
	server := editTestAPI(t, principal("egor"), store)
	defer server.Close()
	response, err := http.Post(server.URL+"/api/sam/products", "application/json",
		strings.NewReader(`{"name":"Git","status":"active","licenseMetric":"device"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", response.StatusCode)
	}
}

func TestCreateProductValidation(t *testing.T) {
	store := &fakeStore{}
	server := editTestAPI(t, principal("egor"), store)
	defer server.Close()
	response, err := http.Post(server.URL+"/api/sam/products", "application/json",
		strings.NewReader(`{"name":"Git","status":"bogus","licenseMetric":"device"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", response.StatusCode)
	}
}

func TestUpdateProductNotFound(t *testing.T) {
	store := &fakeStore{productByIDErr: pgx.ErrNoRows}
	server := editTestAPI(t, principal("egor"), store)
	defer server.Close()
	request, err := http.NewRequest(http.MethodPut, server.URL+"/api/sam/products/7", strings.NewReader(`{"name":"Git","status":"active","licenseMetric":"device"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.StatusCode)
	}
}

func TestLicenseDateValidation(t *testing.T) {
	store := &fakeStore{}
	server := editTestAPI(t, principal("egor"), store)
	defer server.Close()
	response, err := http.Post(server.URL+"/api/sam/licenses", "application/json",
		strings.NewReader(`{"productId":1,"kind":"subscription","seatsTotal":5,"validUntil":"31.12.2026"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", response.StatusCode)
	}
}

func TestServiceRejectsDisabledAccount(t *testing.T) {
	svc := &Service{store: &fakeStore{}, editRun: func(ctx context.Context, fn func(Store) error) error {
		return fn(&fakeStore{})
	}}
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{
		AuthKind: "local",
		Account:  db.LocalAccount{Username: "x", Enabled: false},
	})
	if _, err := svc.CreateProduct(ctx, ProductInput{Name: "Git", Status: "active", LicenseMetric: "device"}); err == nil {
		t.Fatal("expected error for disabled account")
	}
}

func TestAuditEditEncodesValues(t *testing.T) {
	var recorded db.SamEditAuditInsertParams
	store := &stubAuditStore{onInsert: func(params db.SamEditAuditInsertParams) error {
		recorded = params
		return nil
	}}
	if err := auditEdit(context.Background(), store, "egor", "product.create", "product", "1", nil, map[string]any{"name": "Git"}); err != nil {
		t.Fatal(err)
	}
	if recorded.Actor != "egor" || recorded.Action != "product.create" {
		t.Fatalf("unexpected audit params: %+v", recorded)
	}
	if !strings.Contains(string(recorded.After), "Git") {
		t.Fatalf("after payload missing name: %s", recorded.After)
	}
	if recorded.Before != nil {
		t.Fatal("nil before must stay nil")
	}
}

func TestEditErrorMapping(t *testing.T) {
	var status huma.StatusError
	if err := editError(context.Background(), "test", ErrConflict); err == nil || !errors.As(err, &status) || status.GetStatus() != http.StatusConflict {
		t.Fatalf("conflict must map to 409")
	}
	if err := editError(context.Background(), "test", ErrValidation); err == nil || !errors.As(err, &status) || status.GetStatus() != http.StatusUnprocessableEntity {
		t.Fatalf("validation must map to 422")
	}
}

type stubAuditStore struct {
	fakeStore
	onInsert func(db.SamEditAuditInsertParams) error
}

func (s *stubAuditStore) SamEditAuditInsert(_ context.Context, params db.SamEditAuditInsertParams) error {
	return s.onInsert(params)
}
