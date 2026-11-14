package plannerblocks

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func RegisterPublic(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-public-planner-blocks", Method: http.MethodGet, Path: "/api/public/planner",
		Summary: "List public team planner blocks", Tags: []string{"Planner"},
	}, func(ctx context.Context, _ *struct{}) (*publicPlannerOutput, error) {
		blocks, err := svc.ListPublic(ctx)
		if err != nil {
			return nil, plannerAPIError(err)
		}
		return &publicPlannerOutput{Body: blocks}, nil
	})
}

// RegisterTeam exposes the full plan behind the authentication middleware.
// Anonymous visitors only ever receive the compact public projection.
func RegisterTeam(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-team-planner-blocks", Method: http.MethodGet, Path: "/api/planner",
		Summary: "List the full team planner (members only)", Tags: []string{"Planner"},
	}, func(ctx context.Context, _ *struct{}) (*plannerOutput, error) {
		blocks, err := svc.List(ctx)
		if err != nil {
			return nil, plannerAPIError(err)
		}
		return &plannerOutput{Body: blocks}, nil
	})
}

func RegisterAdmin(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "create-planner-block", Method: http.MethodPost, Path: "/api/admin/planner/blocks",
		Summary: "Create a planner block", Tags: []string{"Planner"}, MaxBodyBytes: 32 * 1024,
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *createPlannerBlockInput) (*plannerBlockOutput, error) {
		block, err := svc.Create(ctx, in.Body)
		if err != nil {
			return nil, plannerAPIError(err)
		}
		return &plannerBlockOutput{Body: block}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "update-planner-block", Method: http.MethodPut, Path: "/api/admin/planner/blocks/{id}",
		Summary: "Update a planner block", Tags: []string{"Planner"}, MaxBodyBytes: 32 * 1024,
	}, func(ctx context.Context, in *updatePlannerBlockInput) (*plannerBlockOutput, error) {
		block, err := svc.Update(ctx, in.ID, in.Body)
		if err != nil {
			return nil, plannerAPIError(err)
		}
		return &plannerBlockOutput{Body: block}, nil
	})
	huma.Register(api, huma.Operation{
		OperationID: "delete-planner-block", Method: http.MethodDelete, Path: "/api/admin/planner/blocks/{id}",
		Summary: "Delete a planner block", Tags: []string{"Planner"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *deletePlannerBlockInput) (*struct{}, error) {
		if err := svc.Delete(ctx, in.ID); err != nil {
			return nil, plannerAPIError(err)
		}
		return nil, nil
	})
}

func Register(api huma.API, svc *Service) {
	RegisterPublic(api, svc)
	RegisterTeam(api, svc)
	RegisterAdmin(api, svc)
}

func plannerAPIError(err error) error {
	var statusError huma.StatusError
	if errors.As(err, &statusError) {
		return err
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound("planner.not_found")
	case errors.Is(err, ErrOwnerNotFound):
		return huma.Error400BadRequest("planner.owner_not_found")
	case errors.Is(err, ErrPositionConflict):
		return huma.Error409Conflict("planner.position_conflict")
	case errors.Is(err, ErrInvalidInput):
		return huma.Error422UnprocessableEntity("planner.invalid_input")
	default:
		return huma.Error503ServiceUnavailable("planner.storage_unavailable")
	}
}
