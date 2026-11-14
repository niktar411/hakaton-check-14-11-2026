package compliance

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type summaryOutput struct {
	Body Summary
}

type productsOutput struct {
	Body []ComplianceProduct
}

type risksOutput struct {
	Body []Risk
}

type productOutput struct {
	Body ComplianceProductDetail
}

type hostOutput struct {
	Body HostDetail
}

type productInput struct {
	ID int64 `path:"id" minimum:"1" doc:"Catalog product identifier"`
}

type hostInput struct {
	ID int64 `path:"id" minimum:"1" doc:"Managed host identifier"`
}

// RegisterPublic exposes the read-only compliance projection used by the
// dashboard and the compliance table. It never accepts writes or prices.
func RegisterPublic(api huma.API, svc *Service) {
	register(api, "/api/public/sam/compliance", "sam-compliance", svc)
}

// RegisterTeam exposes the same read-only projection under the authenticated
// team prefix. The contract is identical to the public dashboard endpoints.
func RegisterTeam(api huma.API, svc *Service) {
	register(api, "/api/sam/compliance", "sam-compliance-team", svc)
}

func register(api huma.API, prefix, operationPrefix string, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: operationPrefix + "-summary",
		Method:      http.MethodGet,
		Path:        prefix + "/summary",
		Summary:     "Get SAM compliance KPIs and recommendations",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*summaryOutput, error) {
		summary, err := svc.Summary(ctx)
		if err != nil {
			return nil, complianceError(ctx, "summary", err)
		}
		return &summaryOutput{Body: summary}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: operationPrefix + "-products",
		Method:      http.MethodGet,
		Path:        prefix + "/products",
		Summary:     "List the purchased/installed balance by product",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*productsOutput, error) {
		products, err := svc.Products(ctx)
		if err != nil {
			return nil, complianceError(ctx, "products", err)
		}
		return &productsOutput{Body: products}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: operationPrefix + "-risks",
		Method:      http.MethodGet,
		Path:        prefix + "/risks",
		Summary:     "List compliance risks ordered by severity and cost",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*risksOutput, error) {
		risks, err := svc.Risks(ctx)
		if err != nil {
			return nil, complianceError(ctx, "risks", err)
		}
		return &risksOutput{Body: risks}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: operationPrefix + "-product",
		Method:      http.MethodGet,
		Path:        prefix + "/products/{id}",
		Summary:     "Get one product with licenses and affected hosts",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, in *productInput) (*productOutput, error) {
		detail, err := svc.Product(ctx, in.ID)
		if err != nil {
			return nil, complianceError(ctx, "product", err)
		}
		return &productOutput{Body: detail}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: operationPrefix + "-host",
		Method:      http.MethodGet,
		Path:        prefix + "/hosts/{id}",
		Summary:     "Get one host with its installations and license state",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, in *hostInput) (*hostOutput, error) {
		detail, err := svc.Host(ctx, in.ID)
		if err != nil {
			return nil, complianceError(ctx, "host", err)
		}
		return &hostOutput{Body: detail}, nil
	})
}

func complianceError(ctx context.Context, operation string, err error) error {
	if errors.Is(err, ErrNotFound) {
		return huma.Error404NotFound("sam.not_found")
	}
	slog.ErrorContext(ctx, "SAM compliance storage operation failed", "operation", operation, "error", err)
	return huma.Error503ServiceUnavailable("sam.storage_unavailable")
}
