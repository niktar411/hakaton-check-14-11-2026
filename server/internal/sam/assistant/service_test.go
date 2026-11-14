package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/llm"
	"hackathon/internal/sam/guard"
)

type fakeModel struct {
	ready    bool
	response llm.Response
	err      error
	complete func(context.Context, llm.Request) (llm.Response, error)
	requests []llm.Request
}

func (f *fakeModel) Ready(context.Context) bool { return f.ready }

func (f *fakeModel) Complete(ctx context.Context, input llm.Request) (llm.Response, error) {
	f.requests = append(f.requests, input)
	if f.complete != nil {
		return f.complete(ctx, input)
	}
	return f.response, f.err
}

type fakeStore struct {
	Store
	conversations []db.SamAiConversation
	messages      []db.SamAiMessage
	products      []db.SamProduct
	balances      []db.SamProductBalancesRow
	installations []db.SamFindInstallationsRow
	licenses      []db.SamLicenseSummaryRow
	risks         []db.SamRiskOverview
	unmatched     []db.SamUnmatchedNamesRow
	overview      db.SamOverviewRow
	nextID        int64
}

func (f *fakeStore) CreateSamAIConversation(_ context.Context, arg db.CreateSamAIConversationParams) (db.SamAiConversation, error) {
	f.nextID++
	row := db.SamAiConversation{ID: f.nextID, Actor: arg.Actor, Mode: arg.Mode}
	f.conversations = append(f.conversations, row)
	return row, nil
}

func (f *fakeStore) GetSamAIConversation(_ context.Context, arg db.GetSamAIConversationParams) (db.SamAiConversation, error) {
	for _, row := range f.conversations {
		if row.ID == arg.ID && row.Actor == arg.Actor && row.Mode == arg.Mode {
			return row, nil
		}
	}
	return db.SamAiConversation{}, pgx.ErrNoRows
}

func (f *fakeStore) ListSamAIConversations(context.Context, db.ListSamAIConversationsParams) ([]db.ListSamAIConversationsRow, error) {
	return nil, nil
}

func (f *fakeStore) ListSamAIMessages(context.Context, int64) ([]db.SamAiMessage, error) {
	return append([]db.SamAiMessage(nil), f.messages...), nil
}

func (f *fakeStore) AddSamAIMessage(_ context.Context, arg db.AddSamAIMessageParams) error {
	f.messages = append(f.messages, db.SamAiMessage{ConversationID: arg.ConversationID, Role: arg.Role, Content: arg.Content, ToolName: arg.ToolName})
	return nil
}

func (f *fakeStore) TouchSamAIConversation(context.Context, int64) error { return nil }
func (f *fakeStore) DeleteEmptySamAIConversations(context.Context, db.DeleteEmptySamAIConversationsParams) error {
	return nil
}

func (f *fakeStore) SearchSamProducts(context.Context, db.SearchSamProductsParams) ([]db.SamProduct, error) {
	return append([]db.SamProduct(nil), f.products...), nil
}

func (f *fakeStore) ListSamProducts(context.Context, db.ListSamProductsParams) ([]db.SamProduct, error) {
	return append([]db.SamProduct(nil), f.products...), nil
}

func (f *fakeStore) SamProductBalances(context.Context, db.SamProductBalancesParams) ([]db.SamProductBalancesRow, error) {
	return append([]db.SamProductBalancesRow(nil), f.balances...), nil
}

func (f *fakeStore) SamFindInstallations(context.Context, db.SamFindInstallationsParams) ([]db.SamFindInstallationsRow, error) {
	return append([]db.SamFindInstallationsRow(nil), f.installations...), nil
}

func (f *fakeStore) SamLicenseSummary(context.Context) ([]db.SamLicenseSummaryRow, error) {
	return append([]db.SamLicenseSummaryRow(nil), f.licenses...), nil
}

func (f *fakeStore) SamTopRisks(context.Context, int32) ([]db.SamRiskOverview, error) {
	return append([]db.SamRiskOverview(nil), f.risks...), nil
}

func (f *fakeStore) SamUnmatchedNames(context.Context, int32) ([]db.SamUnmatchedNamesRow, error) {
	return append([]db.SamUnmatchedNamesRow(nil), f.unmatched...), nil
}

func (f *fakeStore) SamOverview(context.Context) (db.SamOverviewRow, error) {
	return f.overview, nil
}

func (f *fakeStore) SamProductCount(context.Context, db.SamProductCountParams) (int64, error) {
	return int64(len(f.products)), nil
}

func (f *fakeStore) SamInstallationCount(context.Context, db.SamInstallationCountParams) (int64, error) {
	return int64(len(f.installations)), nil
}

func testService(store *fakeStore, model *fakeModel) *Service {
	return newService(store, model, guard.NewLimiter(2, 1), func(ctx context.Context, fn func(Store) error) error {
		return fn(store)
	})
}

func toolResponse(name, arguments string) llm.Response {
	return llm.Response{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{
		ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: name, Arguments: arguments},
	}}}}
}

func answerResponse(content string) llm.Response {
	return llm.Response{Message: llm.Message{Role: "assistant", Content: content}}
}

func TestChatUsesToolAndCitesSource(t *testing.T) {
	store := &fakeStore{products: []db.SamProduct{{ID: 7, Name: "Google Chrome", Vendor: "Google", Status: "active"}}}
	calls := 0
	model := &fakeModel{ready: true, complete: func(_ context.Context, input llm.Request) (llm.Response, error) {
		calls++
		if calls == 1 {
			return toolResponse("search_products", `{"query":"chrome","limit":5}`), nil
		}
		if len(input.Messages) == 0 || input.Messages[len(input.Messages)-1].Role != "tool" {
			t.Fatalf("expected tool result in the second model request")
		}
		return answerResponse("Найден 1 продукт: Google Chrome. Источник: sam_products, search_products."), nil
	}}
	service := testService(store, model)
	answer, err := service.Chat(context.Background(), "niktar411", 0, "Найди Chrome", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Fallback {
		t.Fatal("model answer with a tool call must not use the fallback")
	}
	if !strings.Contains(answer.Answer, "Google Chrome") {
		t.Fatalf("unexpected answer: %q", answer.Answer)
	}
	if len(answer.Sources) != 1 || answer.Sources[0] != "search_products" {
		t.Fatalf("unexpected sources: %v", answer.Sources)
	}
	if len(store.messages) != 2 {
		t.Fatalf("expected user and assistant messages, got %d", len(store.messages))
	}
	if len(model.requests[0].Tools) != 7 {
		t.Fatalf("expected 7 read-only tools, got %d", len(model.requests[0].Tools))
	}
}

func TestChatFallsBackWhenModelDoesNotCallTool(t *testing.T) {
	store := &fakeStore{overview: db.SamOverviewRow{Products: 12, ActiveProducts: 10, Installations: 40, UnmatchedInstallations: 3, ActiveLicenses: 5, LicensedSeats: 50, ExpiredLicenses: 1}}
	model := &fakeModel{ready: true, response: answerResponse("Придуманный ответ без инструментов")}
	service := testService(store, model)
	answer, err := service.Chat(context.Background(), "niktar411", 0, "Что у нас с SAM?", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if !answer.Fallback {
		t.Fatal("expected keyword fallback when the model does not call a tool")
	}
	if strings.Contains(answer.Answer, "Придуманный") {
		t.Fatal("ungrounded model answer must not leak into the fallback")
	}
	for _, number := range []string{"12", "40", "3", "50"} {
		if !strings.Contains(answer.Answer, number) {
			t.Fatalf("fallback answer lost number %s: %q", number, answer.Answer)
		}
	}
}

func TestChatFallsBackWhenModelIsUnavailable(t *testing.T) {
	store := &fakeStore{risks: []db.SamRiskOverview{{Risk: "unmatched_installations", Severity: "high", Affected: 4, Detail: "Установки без сопоставленного продукта"}}}
	model := &fakeModel{ready: false}
	service := testService(store, model)
	answer, err := service.Chat(context.Background(), "niktar411", 0, "Какие риски?", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if !answer.Fallback || !strings.Contains(answer.Answer, "4") || !strings.Contains(answer.Answer, "top_risks") {
		t.Fatalf("unexpected fallback answer: %+v", answer)
	}
	if len(model.requests) != 0 {
		t.Fatal("model must not be called when it is not configured")
	}
}

func TestChatStopsAfterFourRounds(t *testing.T) {
	store := &fakeStore{overview: db.SamOverviewRow{Products: 1}}
	model := &fakeModel{ready: true, complete: func(context.Context, llm.Request) (llm.Response, error) {
		return toolResponse("list_products", `{"limit":5}`), nil
	}}
	service := testService(store, model)
	answer, err := service.Chat(context.Background(), "niktar411", 0, "Список", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if !answer.Fallback {
		t.Fatal("expected fallback after the bounded tool loop")
	}
	if len(model.requests) != 4 {
		t.Fatalf("expected exactly four model rounds, got %d", len(model.requests))
	}
}

func TestFallbackAnswersCommonQuestions(t *testing.T) {
	store := &fakeStore{
		products:      []db.SamProduct{{ID: 1, Name: "Google Chrome", Vendor: "Google", Status: "active"}},
		balances:      []db.SamProductBalancesRow{{ID: 1, Name: "Google Chrome", LicensedSeats: 10, Installed: 8, AvailableSeats: 2}},
		installations: []db.SamFindInstallationsRow{{ID: 1, RawName: "Google Chrome", ProductName: "Google Chrome", Version: "120", Status: "installed"}},
		licenses:      []db.SamLicenseSummaryRow{{LicenseType: "subscription", Licenses: 2, Seats: 20, Expired: 1, Products: 1, AnnualCost: 1000}},
		risks:         []db.SamRiskOverview{{Risk: "expired_licenses", Severity: "high", Affected: 1, Detail: "Активные лицензии с истёкшим сроком"}},
		unmatched:     []db.SamUnmatchedNamesRow{{RawName: "chromium", Installations: 3, Devices: 2}},
		overview:      db.SamOverviewRow{Products: 1, ActiveProducts: 1, Installations: 8, LicensedSeats: 20},
	}
	service := testService(store, &fakeModel{ready: false})
	cases := []struct {
		prompt string
		want   string
	}{
		{"Сколько мест осталось у Google Chrome?", "свободно 2"},
		{"Сводка лицензий", "1000"},
		{"Какие риски?", "1 строк"},
		{"Покажи несопоставленные имена", "chromium"},
		{"Где установлен Google Chrome?", "Обезличенные установки по продукту «Google Chrome»"},
		{"Дай общую сводку", "продуктов — 1"},
	}
	for _, tc := range cases {
		answer, _, err := service.fallback(context.Background(), tc.prompt)
		if err != nil {
			t.Fatalf("%s: %v", tc.prompt, err)
		}
		if !strings.Contains(answer, tc.want) {
			t.Fatalf("%s: expected %q in %q", tc.prompt, tc.want, answer)
		}
	}
}

func TestConversationNotFound(t *testing.T) {
	service := testService(&fakeStore{}, &fakeModel{})
	_, err := service.Conversation(context.Background(), "niktar411", 42)
	requireStatus(t, err, 404)
}

func requireStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected HTTP %d, got nil", want)
	}
	var status interface{ GetStatus() int }
	if !errors.As(err, &status) {
		t.Fatalf("expected status error, got %v", err)
	}
	if status.GetStatus() != want {
		t.Fatalf("expected HTTP %d, got %d", want, status.GetStatus())
	}
}
