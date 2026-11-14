package codingtasks

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// Register adds only authenticated application routes. Authentication is
// enforced again in Service so tests and alternative routers cannot bypass it.
func Register(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{OperationID: "create-coding-task", Method: http.MethodPost, Path: "/api/coding/tasks", Tags: []string{"CodingTasks"}, MaxBodyBytes: 20000}, func(ctx context.Context, in *createCodingTaskInput) (*codingTaskOutput, error) {
		return taskResponse(svc.Create(ctx, in.Body.Request, in.Body.ParentID))
	})
	huma.Register(api, huma.Operation{OperationID: "answer-coding-task", Method: http.MethodPost, Path: "/api/coding/tasks/{id}/answer", Tags: []string{"CodingTasks"}, MaxBodyBytes: 20000}, func(ctx context.Context, in *answerCodingTaskInput) (*codingTaskOutput, error) {
		return taskResponse(svc.Answer(ctx, in.ID, in.Body.Answer))
	})
	huma.Register(api, huma.Operation{OperationID: "confirm-coding-task", Method: http.MethodPost, Path: "/api/coding/tasks/{id}/confirm", Tags: []string{"CodingTasks"}, MaxBodyBytes: 1024}, func(ctx context.Context, in *confirmCodingTaskInput) (*codingTaskOutput, error) {
		confirmation := in.Body.Confirmation
		if strings.TrimSpace(confirmation) == "" {
			confirmation = in.Body.Understood
		}
		return taskResponse(svc.Confirm(ctx, in.ID, confirmation))
	})
	huma.Register(api, huma.Operation{OperationID: "rework-coding-task", Method: http.MethodPost, Path: "/api/coding/tasks/{id}/rework", Tags: []string{"CodingTasks"}, MaxBodyBytes: 20000}, func(ctx context.Context, in *reworkCodingTaskInput) (*codingTaskOutput, error) {
		return taskResponse(svc.Rework(ctx, in.ID, in.Body.Request))
	})
	huma.Register(api, huma.Operation{OperationID: "list-coding-tasks", Method: http.MethodGet, Path: "/api/coding/tasks", Tags: []string{"CodingTasks"}}, func(ctx context.Context, in *listCodingTasksInput) (*codingTaskListOutput, error) {
		items, err := svc.List(ctx, in.Offset)
		if err != nil {
			return nil, taskError(err)
		}
		return &codingTaskListOutput{Body: items}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "get-coding-task", Method: http.MethodGet, Path: "/api/coding/tasks/{id}", Tags: []string{"CodingTasks"}}, func(ctx context.Context, in *codingTaskIDInput) (*codingTaskDetailsOutput, error) {
		return detailsResponse(svc.Get(ctx, in.ID))
	})
	huma.Register(api, huma.Operation{OperationID: "admin-list-coding-tasks", Method: http.MethodGet, Path: "/api/admin/coding/tasks", Tags: []string{"CodingTasks", "Admin"}}, func(ctx context.Context, in *listCodingTasksInput) (*codingTaskListOutput, error) {
		items, err := svc.AdminList(ctx, in.Offset)
		if err != nil {
			return nil, taskError(err)
		}
		return &codingTaskListOutput{Body: items}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "admin-get-coding-task", Method: http.MethodGet, Path: "/api/admin/coding/tasks/{id}", Tags: []string{"CodingTasks", "Admin"}}, func(ctx context.Context, in *codingTaskIDInput) (*codingTaskDetailsOutput, error) {
		return detailsResponse(svc.AdminGet(ctx, in.ID))
	})
}

func taskResponse(task CodingTask, err error) (*codingTaskOutput, error) {
	if err != nil {
		return nil, taskError(err)
	}
	return &codingTaskOutput{Body: task}, nil
}

func detailsResponse(details CodingTaskDetails, err error) (*codingTaskDetailsOutput, error) {
	if err != nil {
		return nil, taskError(err)
	}
	return &codingTaskDetailsOutput{Body: details}, nil
}

func taskError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return huma.Error404NotFound("coding_task.not_found")
	case errors.Is(err, ErrInvalid):
		return huma.Error400BadRequest("coding_task.invalid_input")
	case errors.Is(err, ErrConflict):
		return huma.Error409Conflict("coding_task.invalid_phase")
	case errors.Is(err, ErrBusy):
		return huma.Error429TooManyRequests("coding_task.busy")
	case errors.Is(err, ErrForbidden):
		return huma.Error403Forbidden("coding_task.access_denied")
	default:
		return err
	}
}
