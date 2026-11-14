package assistant

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"hackathon/internal/identity"
)

type Answer struct {
	ID       int64    `json:"id"`
	Answer   string   `json:"answer"`
	Sources  []string `json:"sources"`
	Fallback bool     `json:"fallback"`
}

type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

type Summary struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	MessageCount int64     `json:"messageCount"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type History struct {
	ID       int64     `json:"id"`
	Messages []Message `json:"messages"`
}

type chatInput struct {
	Body struct {
		ID        int64  `json:"id" minimum:"0"`
		Prompt    string `json:"prompt" minLength:"1" maxLength:"16000"`
		RequestID string `json:"requestId" minLength:"16" maxLength:"80" pattern:"^[a-zA-Z0-9-]+$"`
	}
}

type chatOutput struct{ Body Answer }

type conversationsInput struct {
	Limit  int32 `query:"limit" minimum:"1" maximum:"100" default:"50"`
	Offset int32 `query:"offset" minimum:"0" default:"0"`
}

type conversationsOutput struct{ Body []Summary }

type historyInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type historyOutput struct{ Body History }

func Register(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID:  "sam-assistant-chat",
		Method:       http.MethodPost,
		Path:         "/api/sam/assistant/chat",
		Summary:      "Вопрос аналитику SAM по данным инвентаризации",
		Tags:         []string{"SAM assistant"},
		MaxBodyBytes: 70000,
	}, func(ctx context.Context, in *chatInput) (*chatOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		answer, err := s.Chat(ctx, principal.Session.Username, in.Body.ID, in.Body.Prompt, in.Body.RequestID)
		if err != nil {
			return nil, err
		}
		return &chatOutput{Body: answer}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-assistant-conversations",
		Method:      http.MethodGet,
		Path:        "/api/sam/assistant/conversations",
		Summary:     "История диалогов аналитика SAM",
		Tags:        []string{"SAM assistant"},
	}, func(ctx context.Context, in *conversationsInput) (*conversationsOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		items, err := s.Conversations(ctx, principal.Session.Username, in.Limit, in.Offset)
		if err != nil {
			return nil, err
		}
		return &conversationsOutput{Body: items}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-assistant-conversation",
		Method:      http.MethodGet,
		Path:        "/api/sam/assistant/conversations/{id}",
		Summary:     "Один диалог аналитика SAM",
		Tags:        []string{"SAM assistant"},
	}, func(ctx context.Context, in *historyInput) (*historyOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		history, err := s.Conversation(ctx, principal.Session.Username, in.ID)
		if err != nil {
			return nil, err
		}
		return &historyOutput{Body: history}, nil
	})
}
