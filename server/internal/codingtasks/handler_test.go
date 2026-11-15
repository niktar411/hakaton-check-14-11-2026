package codingtasks

import (
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

func TestRoutesRequireAuthentication(t *testing.T) {
	_, api := humatest.New(t)
	Register(api, NewService(nil, nil, nil, nil, Config{}))
	for _, route := range []string{
		"/api/coding/tasks",
		"/api/coding/tasks/1",
		"/api/admin/coding/tasks",
		"/api/admin/coding/tasks/1",
	} {
		if response := api.Get(route); response.Code != http.StatusUnauthorized {
			t.Errorf("GET %s: status %d, want 401; body %s", route, response.Code, response.Body.String())
		}
	}
	if response := api.Get("/api/coding/tasks/0"); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid path ID: status %d, want 422", response.Code)
	}
	if response := api.Get("/api/coding-tasks/1"); response.Code != http.StatusNotFound {
		t.Fatalf("generated skeleton route still registered: status %d", response.Code)
	}
}
