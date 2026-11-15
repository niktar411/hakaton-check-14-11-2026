package {{pkg}}

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Register подключает операции ресурса к API.
func Register(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "get-{{kebab}}",
		Method:      http.MethodGet,
		Path:        "/api/{{route}}/{id}",
		Summary:     "{{Type}} по идентификатору",
		Tags:        []string{"{{Type}}"},
	}, func(ctx context.Context, in *get{{Type}}Input) (*{{type}}Output, error) {
		item, err := svc.Get(ctx, in.ID)
		if errors.Is(err, ErrNotFound) {
			return nil, huma.Error404NotFound("{{kebab}}.not_found")
		}
		if err != nil {
			return nil, err
		}
		return &{{type}}Output{Body: item}, nil
	})
}
