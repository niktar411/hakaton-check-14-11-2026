package operations

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"hackathon/internal/codingtasks"
)

type chatAgentInput struct {
	Body struct {
		Prompt   string `json:"prompt" minLength:"1" maxLength:"16000"`
		ParentID *int64 `json:"parentId,omitempty" minimum:"1"`
	}
}

type chatAgentOutput struct{ Body codingtasks.CodingTask }

// ChatAgent turns a chat prompt into a coding task and starts the existing
// coding agent. The agent works in its isolated workspace, runs checks, pushes
// a branch and opens a merge request; it never merges or deploys directly.
func (s *Service) ChatAgent(ctx context.Context, input *chatAgentInput) (*chatAgentOutput, error) {
	identity, err := s.Identity(ctx)
	if err != nil {
		return nil, err
	}
	if !s.model.Ready(ctx) {
		return nil, huma.Error503ServiceUnavailable("model.not_configured")
	}
	release, err := s.acquireChatSlot(identity.Username)
	if err != nil {
		return nil, err
	}
	defer release()
	task, err := s.coding.Create(ctx, input.Body.Prompt, input.Body.ParentID)
	if err != nil {
		return nil, codingTaskError(err)
	}
	task, err = s.coding.Answer(ctx, task.ID, "Действуй по запросу. Все требования уже в запросе; если чего-то не хватает, прими разумные допущения и перечисли их в описании MR.")
	if err != nil {
		return nil, codingTaskError(err)
	}
	task, err = s.coding.Confirm(ctx, task.ID, "подтверждаю")
	if err != nil {
		return nil, codingTaskError(err)
	}
	return &chatAgentOutput{Body: task}, nil
}

func codingTaskError(err error) error {
	switch {
	case errors.Is(err, codingtasks.ErrNotFound):
		return huma.Error404NotFound("coding_task.not_found")
	case errors.Is(err, codingtasks.ErrInvalid):
		return huma.Error400BadRequest("coding_task.invalid_input")
	case errors.Is(err, codingtasks.ErrConflict):
		return huma.Error409Conflict("coding_task.invalid_phase")
	case errors.Is(err, codingtasks.ErrBusy):
		return huma.Error429TooManyRequests("coding_task.busy")
	case errors.Is(err, codingtasks.ErrForbidden):
		return huma.Error403Forbidden("coding_task.access_denied")
	default:
		return huma.Error503ServiceUnavailable("coding_task.unavailable")
	}
}
