package questions

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/identity"
	"hackathon/internal/llm"
)

const (
	testAdminUser = "niktar411"
	testAdminIP   = "10.77.0.2"
	validJSON     = `{"answer":"Grounded answer.","links":[]}`
)

type fakeModel struct {
	response llm.Response
	err      error
	requests []llm.Request
	complete func(context.Context, llm.Request) (llm.Response, error)
}

func (f *fakeModel) Complete(ctx context.Context, input llm.Request) (llm.Response, error) {
	f.requests = append(f.requests, input)
	if f.complete != nil {
		return f.complete(ctx, input)
	}
	return f.response, f.err
}

type fakeStore struct {
	grounding       string
	groundingExists bool
	getSettingErr   error
	upsertErr       error
	addErr          error
	listErr         error
	getCalls        int
	upserted        []db.UpsertApplicationSettingParams
	added           []db.AddFAQEntryParams
	historyRows     []db.FaqEntry
	listParams      []db.ListFAQEntriesParams
}

func (f *fakeStore) GetApplicationSetting(_ context.Context, key string) (db.ApplicationSetting, error) {
	f.getCalls++
	if f.getSettingErr != nil {
		return db.ApplicationSetting{}, f.getSettingErr
	}
	if !f.groundingExists {
		return db.ApplicationSetting{}, pgx.ErrNoRows
	}
	return db.ApplicationSetting{Key: key, Value: f.grounding}, nil
}

func (f *fakeStore) UpsertApplicationSetting(_ context.Context, arg db.UpsertApplicationSettingParams) (db.ApplicationSetting, error) {
	f.upserted = append(f.upserted, arg)
	if f.upsertErr != nil {
		return db.ApplicationSetting{}, f.upsertErr
	}
	f.grounding = arg.Value
	f.groundingExists = true
	return db.ApplicationSetting{Key: arg.Key, Value: arg.Value}, nil
}

func (f *fakeStore) AddFAQEntry(_ context.Context, arg db.AddFAQEntryParams) (db.FaqEntry, error) {
	f.added = append(f.added, arg)
	if f.addErr != nil {
		return db.FaqEntry{}, f.addErr
	}
	return db.FaqEntry{ID: 1, Question: arg.Question, Answer: arg.Answer, Links: arg.Links}, nil
}

func (f *fakeStore) ListFAQEntries(_ context.Context, arg db.ListFAQEntriesParams) ([]db.FaqEntry, error) {
	f.listParams = append(f.listParams, arg)
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]db.FaqEntry(nil), f.historyRows...), nil
}

func testConfig() Config {
	return Config{
		PublicBaseURL:  "https://portal.example/",
		PlannerBaseURL: "https://planner.example/",
		AdminUser:      testAdminUser,
		AdminIP:        testAdminIP,
	}
}

func newUnitService(store *fakeStore, model *fakeModel) *Service {
	return newService(store, model, testConfig())
}

func modelReturning(content string) *fakeModel {
	return &fakeModel{response: llm.Response{Message: llm.Message{Role: "assistant", Content: content}}}
}

func publicContext(ctx context.Context, ip string) context.Context {
	return identity.WithRequestIP(ctx, ip)
}

func adminContext(ctx context.Context) context.Context {
	return identity.WithPrincipal(ctx, identity.Principal{
		AuthKind: "local",
		IP:       testAdminIP,
		Session:  db.Session{Username: testAdminUser},
		Account: db.LocalAccount{
			Username: testAdminUser,
			Role:     "admin",
			Enabled:  true,
		},
	})
}

func nonAdminContext(ctx context.Context) context.Context {
	return identity.WithPrincipal(ctx, identity.Principal{
		AuthKind: "local",
		IP:       testAdminIP,
		Session:  db.Session{Username: "member"},
		Account: db.LocalAccount{
			Username: "member",
			Role:     "member",
			Enabled:  true,
		},
	})
}

type statusCoder interface {
	GetStatus() int
}

func requireStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected HTTP status %d, got nil error", want)
	}
	var status statusCoder
	if !errors.As(err, &status) {
		t.Fatalf("expected HTTP status %d, got non-status error %v", want, err)
	}
	if status.GetStatus() != want {
		t.Fatalf("expected HTTP status %d, got %d (%v)", want, status.GetStatus(), err)
	}
}

func validPublicBody() map[string]string {
	return map[string]string{
		"question":  "What is supported?",
		"requestId": "0123456789abcdef",
	}
}
