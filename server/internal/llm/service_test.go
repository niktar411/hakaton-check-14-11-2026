package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
)

type recordingStore struct {
	settings map[string]string
	begin    db.BeginUsageParams
	finish   db.FinishUsageParams
}

func (s *recordingStore) GetApplicationSetting(_ context.Context, key string) (db.ApplicationSetting, error) {
	value, ok := s.settings[key]
	if !ok {
		return db.ApplicationSetting{}, pgx.ErrNoRows
	}
	return db.ApplicationSetting{Key: key, Value: value}, nil
}

func (s *recordingStore) UpsertApplicationSetting(_ context.Context, arg db.UpsertApplicationSettingParams) (db.ApplicationSetting, error) {
	s.settings[arg.Key] = arg.Value
	return db.ApplicationSetting{Key: arg.Key, Value: arg.Value}, nil
}

func (s *recordingStore) BeginUsage(_ context.Context, arg db.BeginUsageParams) (int64, error) {
	s.begin = arg
	return 42, nil
}

func (s *recordingStore) FinishUsage(_ context.Context, arg db.FinishUsageParams) error {
	s.finish = arg
	return nil
}

func TestCompleteRecordsActualUsageWithoutMetadataInProviderPayload(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-key" {
			t.Error("provider credential missing")
		}
		if r.Header.Get("x-opencode-session") != "chat-17" {
			t.Error("stable provider session header missing")
		}
		if r.Header.Get("User-Agent") != "hackathon-portal/1.0" {
			t.Error("provider user agent missing")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if _, ok := payload["Actor"]; ok {
			t.Error("internal actor sent to provider")
		}
		if _, ok := payload["RequestID"]; ok {
			t.Error("internal request ID sent to provider")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "answer"}}},
			"usage":   map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		}); err != nil {
			t.Error(err)
		}
	}))
	defer provider.Close()
	store := &recordingStore{settings: map[string]string{}}
	service := newService(store, Config{URL: provider.URL, APIKey: "provider-key", Model: "deepseek-test"}, provider.Client())
	response, err := service.Complete(t.Context(), Request{Messages: []Message{{Role: "user", Content: "question"}}, Actor: "public", RequestID: "faq-test-request", Session: "chat-17"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.Content != "answer" || store.begin.Username != "public" || store.finish.ID != 42 || store.finish.Status != "completed" {
		t.Fatalf("unexpected response or usage: %+v %+v %+v", response, store.begin, store.finish)
	}
	if store.finish.InputTokens.Int64 != 11 || store.finish.OutputTokens.Int64 != 7 || store.finish.TotalTokens.Int64 != 18 {
		t.Fatalf("wrong token usage: %+v", store.finish)
	}
}

func TestStreamCollectsReasoningAndContent(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["stream"] != true {
			t.Error("streaming was not requested")
		}
		options, ok := payload["stream_options"].(map[string]any)
		if !ok || options["include_usage"] != true {
			t.Error("usage was not requested in the stream")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`data: {"choices":[{"delta":{"role":"assistant","content":""}}]}`,
			`data: {"choices":[{"delta":{"reasoning_content":"Let me "}}]}`,
			`data: {"choices":[{"delta":{"reasoning_content":"think."}}]}`,
			`data: {"choices":[{"delta":{"content":"The answer"}}]}`,
			`data: {"choices":[{"delta":{"content":" is 4."}}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`,
			`data: [DONE]`,
		}
		for _, frame := range frames {
			if _, err := w.Write([]byte(frame + "\n\n")); err != nil {
				t.Error(err)
			}
		}
	}))
	defer provider.Close()
	store := &recordingStore{settings: map[string]string{}}
	service := newService(store, Config{URL: provider.URL, APIKey: "provider-key", Model: "deepseek-test"}, provider.Client())
	var reasoning, content strings.Builder
	response, err := service.Stream(t.Context(), Request{Messages: []Message{{Role: "user", Content: "question"}}}, func(delta Delta) error {
		reasoning.WriteString(delta.Reasoning)
		content.WriteString(delta.Content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reasoning.String() != "Let me think." || content.String() != "The answer is 4." {
		t.Fatalf("unexpected deltas: %q / %q", reasoning.String(), content.String())
	}
	if response.Message.Content != "The answer is 4." || response.Usage == nil || response.Usage.TotalTokens == nil || *response.Usage.TotalTokens != 8 {
		t.Fatalf("unexpected aggregated response: %+v", response)
	}
}

func TestAPIKeyIsStoredEncryptedAndUsed(t *testing.T) {
	store := &recordingStore{settings: map[string]string{}}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	service := newService(store, Config{URL: "https://foundation-models.api.cloud.ru/v1", Model: "deepseek-test", EncryptionKey: key}, nil)
	const apiKey = "provider-key-with-sufficient-length"
	if err := service.SetAPIKey(t.Context(), apiKey); err != nil {
		t.Fatal(err)
	}
	stored := store.settings[modelKeySetting]
	if stored == apiKey || strings.Contains(stored, apiKey) || stored == "" {
		t.Fatal("model key stored as plaintext")
	}
	_, decrypted, model, err := service.credentials(t.Context())
	if err != nil || decrypted != apiKey || model != "deepseek-test" {
		t.Fatalf("encrypted key not usable: %v", err)
	}
}
