package sam

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type productWriteInput struct {
	ID     int64        `path:"id"`
	Body   ProductInput `json:"body"`
}

type productCreateInput struct {
	Body ProductInput `json:"body"`
}

type productOutput struct {
	Body Product
}

type productIDInput struct {
	ID int64 `path:"id"`
}

type licenseIDInput struct {
	ID int64 `path:"id"`
}

type installationIDInput struct {
	ID int64 `path:"id"`
}

type okOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

type licenseWriteInput struct {
	ID   int64        `path:"id"`
	Body LicenseInput `json:"body"`
}

type licenseCreateInput struct {
	Body LicenseInput `json:"body"`
}

type licenseOutput struct {
	Body License
}

type installationWriteInput struct {
	ID   int64              `path:"id"`
	Body InstallationInput `json:"body"`
}

type installationCreateInput struct {
	Body InstallationInput `json:"body"`
}

type installationOutput struct {
	Body Installation
}

type aliasCreateInput struct {
	ProductID int64      `path:"id"`
	Body      AliasInput `json:"body"`
}

type aliasDeleteInput struct {
	ID int64 `path:"id"`
}

type aliasesListInput struct {
	ID int64 `path:"id"`
}

type aliasesOutput struct {
	Body struct {
		Aliases []Alias `json:"aliases"`
	}
}

// RegisterEdit exposes the authenticated SAM editing surface. The surrounding
// chi group applies the session middleware; requireEditor in the service
// rejects disabled or foreign sessions.
func RegisterEdit(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "sam-create-product", Method: http.MethodPost, Path: "/api/sam/products",
		Summary: "Create a SAM catalog product", Tags: []string{"SAM"},
		MaxBodyBytes: 16 * 1024,
	}, func(ctx context.Context, input *productCreateInput) (*productOutput, error) {
		product, err := svc.CreateProduct(ctx, input.Body)
		if err != nil {
			return nil, editError(ctx, "sam.product.create", err)
		}
		return &productOutput{Body: product}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-update-product", Method: http.MethodPut, Path: "/api/sam/products/{id}",
		Summary: "Update a SAM catalog product", Tags: []string{"SAM"},
		MaxBodyBytes: 16 * 1024,
	}, func(ctx context.Context, input *productWriteInput) (*productOutput, error) {
		product, err := svc.UpdateProduct(ctx, input.ID, input.Body)
		if err != nil {
			return nil, editError(ctx, "sam.product.update", err)
		}
		return &productOutput{Body: product}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-delete-product", Method: http.MethodDelete, Path: "/api/sam/products/{id}",
		Summary: "Delete an unused SAM catalog product", Tags: []string{"SAM"},
	}, func(ctx context.Context, input *productIDInput) (*okOutput, error) {
		err := svc.DeleteProduct(ctx, input.ID)
		return deleteResult(ctx, "sam.product.delete", err)
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-create-license", Method: http.MethodPost, Path: "/api/sam/licenses",
		Summary: "Create a SAM license", Tags: []string{"SAM"},
		MaxBodyBytes: 16 * 1024,
	}, func(ctx context.Context, input *licenseCreateInput) (*licenseOutput, error) {
		license, err := svc.CreateLicense(ctx, input.Body)
		if err != nil {
			return nil, editError(ctx, "sam.license.create", err)
		}
		return &licenseOutput{Body: license}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-update-license", Method: http.MethodPut, Path: "/api/sam/licenses/{id}",
		Summary: "Update a SAM license", Tags: []string{"SAM"},
		MaxBodyBytes: 16 * 1024,
	}, func(ctx context.Context, input *licenseWriteInput) (*licenseOutput, error) {
		license, err := svc.UpdateLicense(ctx, input.ID, input.Body)
		if err != nil {
			return nil, editError(ctx, "sam.license.update", err)
		}
		return &licenseOutput{Body: license}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-delete-license", Method: http.MethodDelete, Path: "/api/sam/licenses/{id}",
		Summary: "Delete a SAM license", Tags: []string{"SAM"},
	}, func(ctx context.Context, input *licenseIDInput) (*okOutput, error) {
		err := svc.DeleteLicense(ctx, input.ID)
		return deleteResult(ctx, "sam.license.delete", err)
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-create-installation", Method: http.MethodPost, Path: "/api/sam/installations",
		Summary: "Record a manual SAM installation", Tags: []string{"SAM"},
		MaxBodyBytes: 16 * 1024,
	}, func(ctx context.Context, input *installationCreateInput) (*installationOutput, error) {
		installation, err := svc.CreateInstallation(ctx, input.Body)
		if err != nil {
			return nil, editError(ctx, "sam.installation.create", err)
		}
		return &installationOutput{Body: installation}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-update-installation", Method: http.MethodPut, Path: "/api/sam/installations/{id}",
		Summary: "Update a SAM installation row", Tags: []string{"SAM"},
		MaxBodyBytes: 16 * 1024,
	}, func(ctx context.Context, input *installationWriteInput) (*installationOutput, error) {
		installation, err := svc.UpdateInstallation(ctx, input.ID, input.Body)
		if err != nil {
			return nil, editError(ctx, "sam.installation.update", err)
		}
		return &installationOutput{Body: installation}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-delete-installation", Method: http.MethodDelete, Path: "/api/sam/installations/{id}",
		Summary: "Delete a SAM installation row", Tags: []string{"SAM"},
	}, func(ctx context.Context, input *installationIDInput) (*okOutput, error) {
		err := svc.DeleteInstallation(ctx, input.ID)
		return deleteResult(ctx, "sam.installation.delete", err)
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-list-aliases", Method: http.MethodGet, Path: "/api/sam/products/{id}/aliases",
		Summary: "List product aliases", Tags: []string{"SAM"},
	}, func(ctx context.Context, input *aliasesListInput) (*aliasesOutput, error) {
		aliases, err := svc.ProductAliases(ctx, input.ID)
		if err != nil {
			return nil, editError(ctx, "sam.alias.list", err)
		}
		body := &aliasesOutput{}
		body.Body.Aliases = aliases
		return body, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-add-alias", Method: http.MethodPost, Path: "/api/sam/products/{id}/aliases",
		Summary: "Add a product alias", Tags: []string{"SAM"},
		MaxBodyBytes: 4 * 1024,
	}, func(ctx context.Context, input *aliasCreateInput) (*aliasesOutput, error) {
		aliases, err := svc.AddAlias(ctx, input.ProductID, input.Body)
		if err != nil {
			return nil, editError(ctx, "sam.alias.create", err)
		}
		body := &aliasesOutput{}
		body.Body.Aliases = aliases
		return body, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "sam-delete-alias", Method: http.MethodDelete, Path: "/api/sam/aliases/{id}",
		Summary: "Delete a product alias", Tags: []string{"SAM"},
	}, func(ctx context.Context, input *aliasDeleteInput) (*okOutput, error) {
		err := svc.DeleteAlias(ctx, input.ID)
		return deleteResult(ctx, "sam.alias.delete", err)
	})
}

func deleteResult(ctx context.Context, operation string, err error) (*okOutput, error) {
	if err != nil {
		return nil, editError(ctx, operation, err)
	}
	body := &okOutput{}
	body.Body.OK = true
	return body, nil
}

func editError(ctx context.Context, operation string, err error) error {
	switch {
	case errors.Is(err, ErrConflict):
		return huma.Error409Conflict("sam.conflict")
	case errors.Is(err, ErrValidation):
		return huma.Error422UnprocessableEntity("sam.invalid")
	}
	var status huma.StatusError
	if errors.As(err, &status) {
		return status
	}
	slog.ErrorContext(ctx, "SAM edit failed", "operation", operation, "error", err)
	return huma.Error503ServiceUnavailable("sam.storage_unavailable")
}
