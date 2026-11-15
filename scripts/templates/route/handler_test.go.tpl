package {{pkg}}

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

type memStore map[int64]{{Type}}

func (m memStore) Get{{Type}}(_ context.Context, id int64) ({{Type}}, error) {
	item, ok := m[id]
	if !ok {
		return {{Type}}{}, ErrNotFound
	}
	return item, nil
}

func TestGet{{Type}}(t *testing.T) {
	_, api := humatest.New(t)
	Register(api, NewService(memStore{1: {ID: 1}}))

	if resp := api.Get("/api/{{route}}/1"); resp.Code != http.StatusOK {
		t.Fatalf("существующая запись: код %d, тело %s", resp.Code, resp.Body.String())
	}
	if resp := api.Get("/api/{{route}}/2"); resp.Code != http.StatusNotFound {
		t.Fatalf("отсутствующая запись: код %d, ожидали 404", resp.Code)
	}
	if resp := api.Get("/api/{{route}}/0"); resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("id=0: код %d, ожидали 422", resp.Code)
	}
}
