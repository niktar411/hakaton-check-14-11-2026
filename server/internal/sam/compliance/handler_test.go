package compliance

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
)

func handlerStore() *fakeStore {
	return &fakeStore{
		summary:  testSummaryRow(),
		products: testProducts(),
		product:  db.SamComplianceProductRow{ID: 1, Name: "Microsoft Office", Status: "active"},
		host:     db.GetSamHostRow{ID: 1, Hostname: "alpha"},
	}
}

func TestPublicComplianceRoutes(t *testing.T) {
	_, api := humatest.New(t)
	RegisterPublic(api, newService(handlerStore()))

	for _, path := range []string{
		"/api/public/sam/compliance/summary",
		"/api/public/sam/compliance/products",
		"/api/public/sam/compliance/risks",
		"/api/public/sam/compliance/products/1",
		"/api/public/sam/compliance/hosts/1",
	} {
		response := api.Get(path)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200; body %s", path, response.Code, response.Body.String())
		}
	}
}

func TestPublicComplianceRoutesMapErrors(t *testing.T) {
	_, api := humatest.New(t)
	store := handlerStore()
	store.productErr = pgx.ErrNoRows
	store.hostErr = pgx.ErrNoRows
	RegisterPublic(api, newService(store))

	for _, path := range []string{
		"/api/public/sam/compliance/products/404",
		"/api/public/sam/compliance/hosts/404",
	} {
		response := api.Get(path)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404; body %s", path, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), "sam.not_found") {
			t.Fatalf("%s: missing error code: %s", path, response.Body.String())
		}
	}

	store = handlerStore()
	store.err = errors.New("database down")
	_, failing := humatest.New(t)
	RegisterPublic(failing, newService(store))
	response := failing.Get("/api/public/sam/compliance/summary")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "sam.storage_unavailable") {
		t.Fatalf("missing public error code: %s", response.Body.String())
	}
}
