package sam

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
	db "hackathon/internal/db/sqlc"
)

func TestPublicRoutes(t *testing.T) {
	_, api := humatest.New(t)
	store := &fakeStore{dashboard: db.SamDashboardRow{
		Products: 1, Assets: 2, Licenses: 3, SeatsTotal: 10, SeatsUsed: 4,
		OpenFindings: 5, PendingMatches: 6, ExpiringLicenses: 7, UnlicensedInstallations: 8,
	}}
	RegisterPublic(api, newService(store))

	cases := []struct {
		path string
		want int
	}{
		{path: "/api/public/sam/dashboard", want: 200},
		{path: "/api/public/sam/catalog", want: 200},
		{path: "/api/public/sam/licenses", want: 200},
		{path: "/api/public/sam/inventory", want: 200},
		{path: "/api/public/sam/compliance", want: 200},
		{path: "/api/public/sam/matching", want: 200},
		{path: "/api/public/sam/assistant", want: 200},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			response := api.Get(tc.path)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d; body %s", response.Code, tc.want, response.Body.String())
			}
		})
	}

	var dashboard Overview
	response := api.Get("/api/public/sam/dashboard")
	if err := json.Unmarshal(response.Body.Bytes(), &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard.SeatsUsed != 4 || dashboard.UnlicensedInstallations != 8 {
		t.Fatalf("unexpected dashboard: %#v", dashboard)
	}
}

func TestPublicRoutesMapStorageErrors(t *testing.T) {
	_, api := humatest.New(t)
	RegisterPublic(api, newService(&fakeStore{err: errors.New("database down")}))

	for _, path := range []string{
		"/api/public/sam/dashboard",
		"/api/public/sam/catalog",
		"/api/public/sam/licenses",
		"/api/public/sam/inventory",
		"/api/public/sam/compliance",
		"/api/public/sam/matching",
		"/api/public/sam/assistant",
	} {
		response := api.Get(path)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status = %d, want 503; body %s", path, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), "sam.storage_unavailable") {
			t.Fatalf("%s: missing public error code: %s", path, response.Body.String())
		}
	}
}
