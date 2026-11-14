package operator

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"hackathon/internal/identity"
)

type SamOperatorDiff struct {
	Target string `json:"target"`
	Field  string `json:"field"`
	Old    any    `json:"old"`
	New    any    `json:"new"`
}

type SamOperatorPreview struct {
	Intent             string            `json:"intent"`
	Summary            string            `json:"summary"`
	Affected           int               `json:"affected"`
	Samples            []map[string]any  `json:"samples"`
	Diffs              []SamOperatorDiff `json:"diffs"`
	Warnings           []string          `json:"warnings"`
	RequiresPhrase     bool              `json:"requiresPhrase"`
	ConfirmationPhrase string            `json:"confirmationPhrase"`
	Limit              int               `json:"limit"`
}

type SamOperatorAction struct {
	ID             int64              `json:"id"`
	ConversationID *int64             `json:"conversationId,omitempty"`
	Actor          string             `json:"actor"`
	Intent         string             `json:"intent"`
	Status         string             `json:"status"`
	Affected       int64              `json:"affected"`
	Error          string             `json:"error,omitempty"`
	Preview        SamOperatorPreview `json:"preview"`
	CreatedAt      time.Time          `json:"createdAt"`
	AppliedAt      *time.Time         `json:"appliedAt,omitempty"`
	UndoneAt       *time.Time         `json:"undoneAt,omitempty"`
	Before         json.RawMessage    `json:"before,omitempty"`
	After          json.RawMessage    `json:"after,omitempty"`
}

type SamOperatorChatResult struct {
	ID       int64               `json:"id"`
	Answer   string              `json:"answer"`
	Actions  []SamOperatorAction `json:"actions"`
	Fallback bool                `json:"fallback"`
}

type samOperatorChatInput struct {
	Body struct {
		ID        int64  `json:"id" minimum:"0"`
		Prompt    string `json:"prompt" minLength:"1" maxLength:"16000"`
		RequestID string `json:"requestId" minLength:"16" maxLength:"80" pattern:"^[a-zA-Z0-9-]+$"`
	}
}

type samOperatorChatOutput struct{ Body SamOperatorChatResult }

type samOperatorActionInput struct {
	ID int64 `path:"id" minimum:"1"`
}

type samOperatorApplyInput struct {
	ID   int64 `path:"id" minimum:"1"`
	Body struct {
		Confirmation string `json:"confirmation" maxLength:"200"`
	}
}

type samOperatorActionOutput struct{ Body SamOperatorAction }

type samOperatorActionsInput struct {
	Limit          int32  `query:"limit" minimum:"1" maximum:"100" default:"50"`
	Offset         int32  `query:"offset" minimum:"0" default:"0"`
	ConversationID int64  `query:"conversationId" minimum:"0" default:"0"`
	Status         string `query:"status" maxLength:"20" default:""`
}

type samOperatorActionsOutput struct{ Body []SamOperatorAction }

func Register(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID:  "sam-operator-chat",
		Method:       http.MethodPost,
		Path:         "/api/sam/operator/chat",
		Summary:      "Предложить изменение SAM через AI-оператора",
		Tags:         []string{"SAM operator"},
		MaxBodyBytes: 70000,
	}, func(ctx context.Context, in *samOperatorChatInput) (*samOperatorChatOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		result, err := s.Chat(ctx, principal.Session.Username, in.Body.ID, in.Body.Prompt, in.Body.RequestID)
		if err != nil {
			return nil, err
		}
		return &samOperatorChatOutput{Body: result}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-operator-apply",
		Method:      http.MethodPost,
		Path:        "/api/sam/operator/actions/{id}/apply",
		Summary:     "Применить подтверждённое изменение SAM",
		Tags:        []string{"SAM operator"},
	}, func(ctx context.Context, in *samOperatorApplyInput) (*samOperatorActionOutput, error) {
		principal, err := identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP)
		if err != nil {
			return nil, err
		}
		action, err := s.Apply(ctx, principal.Session.Username, true, in.ID, in.Body.Confirmation)
		if err != nil {
			return nil, err
		}
		return &samOperatorActionOutput{Body: action}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-operator-cancel",
		Method:      http.MethodPost,
		Path:        "/api/sam/operator/actions/{id}/cancel",
		Summary:     "Отменить предложенное изменение SAM",
		Tags:        []string{"SAM operator"},
	}, func(ctx context.Context, in *samOperatorActionInput) (*samOperatorActionOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		admin := identity.IsAdmin(principal, s.config.AdminUser, s.config.AdminIP)
		action, err := s.Cancel(ctx, principal.Session.Username, admin, in.ID)
		if err != nil {
			return nil, err
		}
		return &samOperatorActionOutput{Body: action}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-operator-undo",
		Method:      http.MethodPost,
		Path:        "/api/sam/operator/actions/{id}/undo",
		Summary:     "Откатить последнее применённое изменение SAM",
		Tags:        []string{"SAM operator"},
	}, func(ctx context.Context, in *samOperatorActionInput) (*samOperatorActionOutput, error) {
		principal, err := identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP)
		if err != nil {
			return nil, err
		}
		action, err := s.Undo(ctx, principal.Session.Username, in.ID)
		if err != nil {
			return nil, err
		}
		return &samOperatorActionOutput{Body: action}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-operator-actions",
		Method:      http.MethodGet,
		Path:        "/api/sam/operator/actions",
		Summary:     "История предложенных и применённых изменений SAM",
		Tags:        []string{"SAM operator"},
	}, func(ctx context.Context, in *samOperatorActionsInput) (*samOperatorActionsOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		admin := identity.IsAdmin(principal, s.config.AdminUser, s.config.AdminIP)
		items, err := s.Actions(ctx, principal.Session.Username, admin, in.ConversationID, in.Status, in.Limit, in.Offset)
		if err != nil {
			return nil, err
		}
		return &samOperatorActionsOutput{Body: items}, nil
	})
}
