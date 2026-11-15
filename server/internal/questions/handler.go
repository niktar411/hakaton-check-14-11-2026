package questions

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

const (
	maxPublicBodyBytes = 64 << 10
	maxAdminBodyBytes  = 2 << 20
)

// RegisterPublic registers routes which deliberately do not require a principal.
func RegisterPublic(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID:  "ask-public-question",
		Method:       http.MethodPost,
		Path:         "/api/public/questions",
		Summary:      "Ответ на публичный вопрос по проверенному контексту",
		Tags:         []string{"Questions"},
		MaxBodyBytes: maxPublicBodyBytes,
	}, func(ctx context.Context, in *publicQuestionInput) (*publicQuestionOutput, error) {
		answer, err := s.Ask(ctx, in.Body.Question, in.Body.RequestID)
		if err != nil {
			return nil, err
		}
		return &publicQuestionOutput{Body: answer}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-public-site-config",
		Method:      http.MethodGet,
		Path:        "/api/public/site-config",
		Summary:     "Публичные адреса и разрешённые ссылки",
		Tags:        []string{"Questions"},
	}, func(_ context.Context, _ *struct{}) (*siteConfigOutput, error) {
		return &siteConfigOutput{Body: s.SiteConfig()}, nil
	})
}

// RegisterAdmin registers grounding and history operations; Service enforces admin authorization.
func RegisterAdmin(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "get-questions-context",
		Method:      http.MethodGet,
		Path:        "/api/admin/questions/context",
		Summary:     "Контекст публичных ответов",
		Tags:        []string{"Questions"},
	}, func(ctx context.Context, _ *struct{}) (*contextOutput, error) {
		grounding, err := s.GetContext(ctx)
		if err != nil {
			return nil, err
		}
		return &contextOutput{Body: grounding}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:  "put-questions-context",
		Method:       http.MethodPut,
		Path:         "/api/admin/questions/context",
		Summary:      "Обновить контекст публичных ответов",
		Tags:         []string{"Questions"},
		MaxBodyBytes: maxAdminBodyBytes,
	}, func(ctx context.Context, in *contextInput) (*contextOutput, error) {
		grounding, err := s.SetContext(ctx, in.Body.Context)
		if err != nil {
			return nil, err
		}
		return &contextOutput{Body: grounding}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-questions-history",
		Method:      http.MethodGet,
		Path:        "/api/admin/questions/history",
		Summary:     "История публичных вопросов и ответов",
		Tags:        []string{"Questions"},
	}, func(ctx context.Context, in *historyInput) (*historyOutput, error) {
		history, err := s.History(ctx, in.Limit, in.Offset)
		if err != nil {
			return nil, err
		}
		return &historyOutput{Body: history}, nil
	})
}
