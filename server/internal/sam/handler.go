package sam

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type dashboardOutput struct {
	Body Overview
}

type productsOutput struct {
	Body []Product
}

type licensesOutput struct {
	Body []License
}

type assetsOutput struct {
	Body []Asset
}

type findingsOutput struct {
	Body []Finding
}

type matchesOutput struct {
	Body []Match
}

type assistantOutput struct {
	Body Assistant
}

// RegisterPublic exposes a read-only SAM projection. The section is reachable
// from the public navigation, so these handlers never accept writes.
func RegisterPublic(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "sam-dashboard", Method: http.MethodGet, Path: "/api/public/sam/dashboard",
		Summary: "Get SAM aggregate metrics", Tags: []string{"SAM"},
	}, func(ctx context.Context, _ *struct{}) (*dashboardOutput, error) {
		data, err := svc.Dashboard(ctx)
		if err != nil {
			return nil, samAPIError(ctx, "dashboard", err)
		}
		return &dashboardOutput{Body: data}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-products", Method: http.MethodGet, Path: "/api/public/sam/catalog",
		Summary: "List SAM catalog products", Tags: []string{"SAM"},
	}, func(ctx context.Context, _ *struct{}) (*productsOutput, error) {
		data, err := svc.Products(ctx)
		if err != nil {
			return nil, samAPIError(ctx, "catalog", err)
		}
		return &productsOutput{Body: data}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-licenses", Method: http.MethodGet, Path: "/api/public/sam/licenses",
		Summary: "List SAM licenses", Tags: []string{"SAM"},
	}, func(ctx context.Context, _ *struct{}) (*licensesOutput, error) {
		data, err := svc.Licenses(ctx)
		if err != nil {
			return nil, samAPIError(ctx, "licenses", err)
		}
		return &licensesOutput{Body: data}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-inventory", Method: http.MethodGet, Path: "/api/public/sam/inventory",
		Summary: "List SAM inventory assets", Tags: []string{"SAM"},
	}, func(ctx context.Context, _ *struct{}) (*assetsOutput, error) {
		data, err := svc.Assets(ctx)
		if err != nil {
			return nil, samAPIError(ctx, "inventory", err)
		}
		return &assetsOutput{Body: data}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-compliance", Method: http.MethodGet, Path: "/api/public/sam/compliance",
		Summary: "List SAM compliance findings", Tags: []string{"SAM"},
	}, func(ctx context.Context, _ *struct{}) (*findingsOutput, error) {
		data, err := svc.Findings(ctx)
		if err != nil {
			return nil, samAPIError(ctx, "compliance", err)
		}
		return &findingsOutput{Body: data}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-matching", Method: http.MethodGet, Path: "/api/public/sam/matching",
		Summary: "List SAM matching proposals", Tags: []string{"SAM"},
	}, func(ctx context.Context, _ *struct{}) (*matchesOutput, error) {
		data, err := svc.Matches(ctx)
		if err != nil {
			return nil, samAPIError(ctx, "matching", err)
		}
		return &matchesOutput{Body: data}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-assistant", Method: http.MethodGet, Path: "/api/public/sam/assistant",
		Summary: "Get SAM assistant recommendations", Tags: []string{"SAM"},
	}, func(ctx context.Context, _ *struct{}) (*assistantOutput, error) {
		data, err := svc.Assistant(ctx)
		if err != nil {
			return nil, samAPIError(ctx, "assistant", err)
		}
		return &assistantOutput{Body: data}, nil
	})
}

func samAPIError(ctx context.Context, operation string, err error) error {
	slog.ErrorContext(ctx, "SAM storage operation failed", "operation", operation, "error", err)
	return huma.Error503ServiceUnavailable("sam.storage_unavailable")
}
