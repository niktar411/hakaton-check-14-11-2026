package compliance

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type publicSummaryOutput struct {
	Body SamComplianceSummary
}

type publicProductsOutput struct {
	Body []SamComplianceProduct
}

type publicRisksOutput struct {
	Body []SamComplianceRisk
}

type publicProductOutput struct {
	Body SamComplianceProductDetail
}

type publicHostOutput struct {
	Body SamComplianceHostDetail
}

type publicProductInput struct {
	ID int64 `path:"id" minimum:"1" doc:"Catalog product identifier"`
}

type publicHostInput struct {
	Hostname string `path:"hostname" minLength:"1" doc:"Managed host name"`
}

// RegisterPublic exposes the read-only compliance projection used by the
// public SAM dashboard. It never accepts writes or prices.
func RegisterPublic(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "sam-public-compliance-summary",
		Method:      http.MethodGet,
		Path:        "/api/public/sam/compliance/summary",
		Summary:     "Get public SAM compliance KPIs",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*publicSummaryOutput, error) {
		summary, err := svc.Summary(ctx)
		if err != nil {
			return nil, publicError(err)
		}
		return &publicSummaryOutput{Body: summary}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-public-compliance-products",
		Method:      http.MethodGet,
		Path:        "/api/public/sam/compliance/products",
		Summary:     "List the public purchased/installed balance",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*publicProductsOutput, error) {
		products, err := svc.Products(ctx)
		if err != nil {
			return nil, publicError(err)
		}
		return &publicProductsOutput{Body: products}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-public-compliance-risks",
		Method:      http.MethodGet,
		Path:        "/api/public/sam/compliance/risks",
		Summary:     "List public SAM compliance risks",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, _ *struct{}) (*publicRisksOutput, error) {
		risks, err := svc.Risks(ctx)
		if err != nil {
			return nil, publicError(err)
		}
		return &publicRisksOutput{Body: risks}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-public-compliance-product",
		Method:      http.MethodGet,
		Path:        "/api/public/sam/compliance/products/{id}",
		Summary:     "Get a public product with licenses and hosts",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, in *publicProductInput) (*publicProductOutput, error) {
		detail, err := svc.Product(ctx, in.ID)
		if err != nil {
			return nil, publicError(err)
		}
		return &publicProductOutput{Body: detail}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "sam-public-compliance-host",
		Method:      http.MethodGet,
		Path:        "/api/public/sam/compliance/hosts/{hostname}",
		Summary:     "Get a public host with its installations",
		Tags:        []string{"SAM compliance"},
	}, func(ctx context.Context, in *publicHostInput) (*publicHostOutput, error) {
		detail, err := svc.Host(ctx, in.Hostname)
		if err != nil {
			return nil, publicError(err)
		}
		return &publicHostOutput{Body: detail}, nil
	})
}

func publicError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return huma.Error404NotFound("sam.not_found")
	}
	return err
}
