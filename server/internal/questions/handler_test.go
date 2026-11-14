package questions

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
)

func TestPublicRoutesDoNotRequireAuthentication(t *testing.T) {
	_, api := humatest.New(t)
	store := &fakeStore{groundingExists: true}
	model := modelReturning(validJSON)
	s := newUnitService(store, model)
	RegisterPublic(api, s)

	response := api.Get("/api/public/site-config")
	if response.Code != http.StatusOK {
		t.Fatalf("anonymous site config: status %d, body %s", response.Code, response.Body.String())
	}
	var config SiteConfig
	if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if config.PublicBaseURL != "https://portal.example" || config.PlannerBaseURL != "https://planner.example" {
		t.Fatalf("unexpected public URLs: %#v", config)
	}
	wantRegistry := []Link{
		{Title: "Главная", Path: "/"},
		{Title: "Вопросы и ответы", Path: "/questions"},
		{Title: "Планировщик", Path: "/planner"},
		{Title: "Внешний планировщик", Path: "https://planner.example"},
	}
	if !reflect.DeepEqual(config.Links, wantRegistry) {
		t.Fatalf("link registry = %#v, want %#v", config.Links, wantRegistry)
	}

	// The context intentionally has a request IP but no identity.Principal.
	response = api.PostCtx(publicContext(t.Context(), "203.0.113.70"), "/api/public/questions", validPublicBody())
	if response.Code != http.StatusOK {
		t.Fatalf("anonymous public question: status %d, body %s", response.Code, response.Body.String())
	}
	var answer Answer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Answer != "Grounded answer." || answer.Links == nil || len(answer.Links) != 0 {
		t.Fatalf("unexpected public answer: %#v", answer)
	}
	if len(store.added) != 1 || len(model.requests) != 1 {
		t.Fatal("anonymous public request did not execute the real service flow")
	}
}

func TestPublicRouteRequiresTrustedIP(t *testing.T) {
	_, api := humatest.New(t)
	model := modelReturning(validJSON)
	s := newUnitService(&fakeStore{}, model)
	RegisterPublic(api, s)

	response := api.Post("/api/public/questions", validPublicBody())
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body %s", response.Code, response.Body.String())
	}
	if len(model.requests) != 0 {
		t.Fatal("request without trusted IP reached the model")
	}
}

func TestPublicRouteValidationAndBodyLimit(t *testing.T) {
	_, api := humatest.New(t)
	model := modelReturning(validJSON)
	s := newUnitService(&fakeStore{}, model)
	RegisterPublic(api, s)
	ctx := publicContext(t.Context(), "203.0.113.71")

	tests := []struct {
		name string
		body map[string]string
	}{
		{name: "empty question", body: map[string]string{"question": "", "requestId": "0123456789abcdef"}},
		{name: "question too long", body: map[string]string{"question": strings.Repeat("x", maxQuestionLength+1), "requestId": "0123456789abcdef"}},
		{name: "short request ID", body: map[string]string{"question": "Question?", "requestId": "short"}},
		{name: "unsafe request ID", body: map[string]string{"question": "Question?", "requestId": "0123456789abcde/"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := api.PostCtx(ctx, "/api/public/questions", tc.body)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422; body %s", response.Code, response.Body.String())
			}
		})
	}
	if len(model.requests) != 0 {
		t.Fatal("schema-invalid public request reached the model")
	}

	oversized := `{"question":"` + strings.Repeat("x", maxPublicBodyBytes) + `","requestId":"0123456789abcdef"}`
	response := api.PostCtx(ctx, "/api/public/questions", "Content-Type: application/json", strings.NewReader(oversized))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: status %d, want 413; body %s", response.Code, response.Body.String())
	}
	if len(model.requests) != 0 {
		t.Fatal("oversized public request reached the model")
	}
}

func TestAdminRoutesRequireAdministrator(t *testing.T) {
	_, api := humatest.New(t)
	store := &fakeStore{}
	s := newUnitService(store, modelReturning(validJSON))
	RegisterAdmin(api, s)

	for _, request := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{name: "get context", method: http.MethodGet, path: "/api/admin/questions/context"},
		{name: "put context", method: http.MethodPut, path: "/api/admin/questions/context", body: Context{Context: "grounding"}},
		{name: "history", method: http.MethodGet, path: "/api/admin/questions/history"},
	} {
		t.Run(request.name, func(t *testing.T) {
			var responseCode int
			if request.body == nil {
				responseCode = api.Do(request.method, request.path).Code
			} else {
				responseCode = api.Do(request.method, request.path, request.body).Code
			}
			if responseCode != http.StatusUnauthorized {
				t.Fatalf("anonymous status = %d, want 401", responseCode)
			}

			if request.body == nil {
				responseCode = api.DoCtx(nonAdminContext(t.Context()), request.method, request.path).Code
			} else {
				responseCode = api.DoCtx(nonAdminContext(t.Context()), request.method, request.path, request.body).Code
			}
			if responseCode != http.StatusForbidden {
				t.Fatalf("non-admin status = %d, want 403", responseCode)
			}
		})
	}
	if store.getCalls != 0 || len(store.upserted) != 0 || len(store.listParams) != 0 {
		t.Fatal("unauthorized admin request reached storage")
	}
}

func TestAdminContextAndHistoryRoutes(t *testing.T) {
	_, api := humatest.New(t)
	createdAt := time.Date(2026, time.November, 5, 11, 30, 0, 0, time.UTC)
	store := &fakeStore{historyRows: []db.FaqEntry{
		{
			ID:        9,
			Question:  "Stored question",
			Answer:    "Stored answer",
			Links:     []byte(`[{"title":"forged","path":"/coding"},{"title":"evil","path":"https://evil.example"}]`),
			CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
		},
	}}
	s := newUnitService(store, modelReturning(validJSON))
	RegisterAdmin(api, s)
	ctx := adminContext(t.Context())

	response := api.GetCtx(ctx, "/api/admin/questions/context")
	if response.Code != http.StatusOK {
		t.Fatalf("get missing context: status %d, body %s", response.Code, response.Body.String())
	}
	var grounding Context
	if err := json.Unmarshal(response.Body.Bytes(), &grounding); err != nil {
		t.Fatal(err)
	}
	if grounding.Context != "" {
		t.Fatalf("missing grounding = %q, want empty", grounding.Context)
	}

	response = api.PutCtx(ctx, "/api/admin/questions/context", Context{Context: "Administrator-controlled facts"})
	if response.Code != http.StatusOK {
		t.Fatalf("put context: status %d, body %s", response.Code, response.Body.String())
	}
	if len(store.upserted) != 1 || store.upserted[0].Key != groundingSettingKey || store.upserted[0].Value != "Administrator-controlled facts" {
		t.Fatalf("unexpected grounding upsert: %#v", store.upserted)
	}

	response = api.GetCtx(ctx, "/api/admin/questions/history?limit=2&offset=3")
	if response.Code != http.StatusOK {
		t.Fatalf("history: status %d, body %s", response.Code, response.Body.String())
	}
	var history History
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Limit != 2 || history.Offset != 3 || len(history.Items) != 1 {
		t.Fatalf("unexpected history page: %#v", history)
	}
	if len(store.listParams) != 1 || store.listParams[0].Limit != 2 || store.listParams[0].Offset != 3 {
		t.Fatalf("pagination was not passed to sqlc: %#v", store.listParams)
	}
	item := history.Items[0]
	wantLinks := []Link{}
	if item.ID != 9 || item.Question != "Stored question" || item.Answer != "Stored answer" ||
		!item.CreatedAt.Equal(createdAt) || !reflect.DeepEqual(item.Links, wantLinks) {
		t.Fatalf("unexpected history entry: %#v", item)
	}

	response = api.PutCtx(ctx, "/api/admin/questions/context", Context{Context: strings.Repeat("x", maxGroundingLength+1)})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("overlong grounding: status %d, want 422", response.Code)
	}
	if len(store.upserted) != 1 {
		t.Fatal("overlong grounding reached storage")
	}

	response = api.GetCtx(ctx, "/api/admin/questions/history?limit=0")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid pagination: status %d, want 422", response.Code)
	}
	if len(store.listParams) != 1 {
		t.Fatal("invalid pagination reached storage")
	}
}
