package questions

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAskUsesOnlyGroundingAndStoresCanonicalLinks(t *testing.T) {
	store := &fakeStore{
		grounding:       "UNIQUE_ADMIN_GROUNDING: only the blue deployment is supported.",
		groundingExists: true,
	}
	model := modelReturning(`{
		"answer":"Only the blue deployment is supported.",
		"links":[
			{"title":"attacker-controlled title","path":"/questions"},
			{"title":"phishing","path":"https://evil.example/phish"},
			{"title":"planner","path":"https://planner.example"},
			{"title":"duplicate","path":"/questions"},
			{"title":"private","path":"/admin"},
			{"title":"protocol relative","path":"//evil.example"},
			{"title":"script","path":"javascript:alert(1)"},
			{"title":"non-canonical slash","path":"https://planner.example/"}
		]
	}`)
	s := newUnitService(store, model)
	ctx := publicContext(t.Context(), "203.0.113.10")
	const question = "Which deployment is supported?"
	const requestID = "0123456789abcdef"

	answer, err := s.Ask(ctx, question, requestID)
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	wantLinks := []Link{
		{Title: "Вопросы и ответы", Path: "/questions"},
		{Title: "Внешний планировщик", Path: "https://planner.example"},
	}
	if answer.Answer != "Only the blue deployment is supported." || !reflect.DeepEqual(answer.Links, wantLinks) {
		t.Fatalf("unexpected answer: %#v", answer)
	}
	if len(model.requests) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.requests))
	}
	request := model.requests[0]
	if request.ResponseFormat == nil || request.ResponseFormat.Type != "json_object" {
		t.Fatalf("response format = %#v, want json_object", request.ResponseFormat)
	}
	if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
		t.Fatalf("unexpected model messages: %#v", request.Messages)
	}
	if !strings.Contains(request.Messages[0].Content, store.grounding) {
		t.Fatal("administrator grounding was not sent to the model")
	}
	if !strings.Contains(request.Messages[0].Content, "do not invent facts or links") {
		t.Fatal("model instruction does not forbid invented facts and links")
	}
	if request.Messages[1].Content != question {
		t.Fatalf("model question = %q, want %q", request.Messages[1].Content, question)
	}
	for _, message := range request.Messages {
		if strings.Contains(message.Content, requestID) || strings.Contains(message.Content, "203.0.113.10") {
			t.Fatal("request metadata leaked into the model prompt")
		}
	}
	if len(store.added) != 1 {
		t.Fatalf("stored answers = %d, want 1", len(store.added))
	}
	stored := store.added[0]
	if stored.Question != question || stored.Answer != answer.Answer {
		t.Fatalf("stored answer does not match the request and model answer: %#v", stored)
	}
	var storedLinks []Link
	if err := json.Unmarshal(stored.Links, &storedLinks); err != nil {
		t.Fatalf("stored links are not JSON: %v", err)
	}
	if !reflect.DeepEqual(storedLinks, wantLinks) {
		t.Fatalf("stored links = %#v, want %#v", storedLinks, wantLinks)
	}
}

func TestAskRejectsMalformedModelJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "invalid syntax", content: `{"answer":`},
		{name: "markdown fence", content: "```json\n{\"answer\":\"ok\",\"links\":[]}\n```"},
		{name: "unknown top-level field", content: `{"answer":"ok","links":[],"extra":true}`},
		{name: "unknown link field", content: `{"answer":"ok","links":[{"title":"x","path":"/","extra":true}]}`},
		{name: "trailing garbage", content: `{"answer":"ok","links":[]} trailing`},
		{name: "multiple values", content: `{"answer":"ok","links":[]}{"answer":"second","links":[]}`},
		{name: "missing links", content: `{"answer":"ok"}`},
		{name: "missing answer", content: `{"links":[]}`},
		{name: "null links", content: `{"answer":"ok","links":null}`},
		{name: "null answer", content: `{"answer":null,"links":[]}`},
		{name: "wrong answer type", content: `{"answer":42,"links":[]}`},
		{name: "empty answer", content: `{"answer":"  \n","links":[]}`},
		{name: "missing link fields", content: `{"answer":"ok","links":[{}]}`},
		{name: "null link title", content: `{"answer":"ok","links":[{"title":null,"path":"/"}]}`},
		{name: "null link path", content: `{"answer":"ok","links":[{"title":"x","path":null}]}`},
		{name: "wrong links type", content: `{"answer":"ok","links":{}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{groundingExists: true}
			s := newUnitService(store, modelReturning(tc.content))
			_, err := s.Ask(publicContext(t.Context(), "203.0.113.20"), "Question?", "0123456789abcdef")
			requireStatus(t, err, http.StatusBadGateway)
			if len(store.added) != 0 {
				t.Fatal("malformed model output was persisted")
			}
		})
	}
}

func TestAskAllowsAnswerWithoutSupportedLinks(t *testing.T) {
	store := &fakeStore{groundingExists: true}
	model := modelReturning(`{"answer":"The grounding does not contain this information.","links":[{"title":"unknown","path":"https://evil.example"}]}`)
	s := newUnitService(store, model)

	answer, err := s.Ask(publicContext(t.Context(), "203.0.113.30"), "Unknown?", "0123456789abcdef")
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	if answer.Links == nil || len(answer.Links) != 0 {
		t.Fatalf("links = %#v, want a non-nil empty array", answer.Links)
	}
	if len(store.added) != 1 || string(store.added[0].Links) != "[]" {
		t.Fatalf("stored links = %s, want []", store.added[0].Links)
	}
}

func TestAskProviderAndStorageErrorsAreSafe(t *testing.T) {
	t.Run("provider", func(t *testing.T) {
		store := &fakeStore{groundingExists: true}
		model := modelReturning("")
		model.err = errors.New("provider secret must not escape")
		s := newUnitService(store, model)
		_, err := s.Ask(publicContext(t.Context(), "203.0.113.40"), "Question?", "0123456789abcdef")
		requireStatus(t, err, http.StatusBadGateway)
		if strings.Contains(err.Error(), "provider secret") || len(store.added) != 0 {
			t.Fatalf("unsafe provider failure: %v", err)
		}
	})

	t.Run("grounding storage", func(t *testing.T) {
		store := &fakeStore{getSettingErr: errors.New("database secret must not escape")}
		model := modelReturning(validJSON)
		s := newUnitService(store, model)
		_, err := s.Ask(publicContext(t.Context(), "203.0.113.41"), "Question?", "0123456789abcdef")
		requireStatus(t, err, http.StatusServiceUnavailable)
		if strings.Contains(err.Error(), "database secret") || len(model.requests) != 0 {
			t.Fatalf("unsafe storage failure: %v", err)
		}
	})

	t.Run("answer storage", func(t *testing.T) {
		store := &fakeStore{groundingExists: true, addErr: errors.New("insert secret must not escape")}
		s := newUnitService(store, modelReturning(validJSON))
		_, err := s.Ask(publicContext(t.Context(), "203.0.113.42"), "Question?", "0123456789abcdef")
		requireStatus(t, err, http.StatusServiceUnavailable)
		if strings.Contains(err.Error(), "insert secret") {
			t.Fatalf("unsafe insert failure: %v", err)
		}
	})
}

func TestAskValidatesRequestAndRequiresTrustedIP(t *testing.T) {
	tests := []struct {
		name      string
		question  string
		requestID string
	}{
		{name: "empty question", question: "", requestID: "0123456789abcdef"},
		{name: "whitespace question", question: " \n\t", requestID: "0123456789abcdef"},
		{name: "long question", question: strings.Repeat("я", maxQuestionLength+1), requestID: "0123456789abcdef"},
		{name: "short request ID", question: "Question?", requestID: "short"},
		{name: "unsafe request ID", question: "Question?", requestID: "0123456789abcde/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := modelReturning(validJSON)
			s := newUnitService(&fakeStore{}, model)
			_, err := s.Ask(publicContext(t.Context(), "203.0.113.50"), tc.question, tc.requestID)
			requireStatus(t, err, http.StatusUnprocessableEntity)
			if len(model.requests) != 0 {
				t.Fatal("invalid request reached the model")
			}
		})
	}

	model := modelReturning(validJSON)
	s := newUnitService(&fakeStore{}, model)
	_, err := s.Ask(t.Context(), "Question?", "0123456789abcdef")
	requireStatus(t, err, http.StatusServiceUnavailable)
	if len(model.requests) != 0 {
		t.Fatal("request without a trusted IP reached the model")
	}
}

func TestAskRateLimitIsPerIPAndFixedWindow(t *testing.T) {
	store := &fakeStore{groundingExists: true}
	model := modelReturning(validJSON)
	s := newUnitService(store, model)
	s.limiter.limit = 2
	now := time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	ask := func(ip string) error {
		_, err := s.Ask(publicContext(t.Context(), ip), "Question?", "0123456789abcdef")
		return err
	}
	if err := ask("203.0.113.60"); err != nil {
		t.Fatal(err)
	}
	if err := ask("203.0.113.60"); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, ask("203.0.113.60"), http.StatusTooManyRequests)
	if err := ask("203.0.113.61"); err != nil {
		t.Fatalf("another IP shared the first IP's limit: %v", err)
	}

	now = now.Add(publicRateWindow + time.Nanosecond)
	if err := ask("203.0.113.60"); err != nil {
		t.Fatalf("new fixed window did not reset the limit: %v", err)
	}
	if len(model.requests) != 4 {
		t.Fatalf("model calls = %d, want 4 allowed requests", len(model.requests))
	}
}
