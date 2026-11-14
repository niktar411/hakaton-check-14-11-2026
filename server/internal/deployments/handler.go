package deployments

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"hackathon/internal/identity"
)

// Register connects the administrative Huma operations. The public GitLab
// webhook deliberately remains a raw HTTP handler on Service.
func Register(api huma.API, svc *Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-deployments",
		Method:      http.MethodGet,
		Path:        "/api/admin/deployments",
		Summary:     "List deployment operations",
		Tags:        []string{"Deployments"},
	}, func(ctx context.Context, in *deploymentListInput) (*deploymentListOutput, error) {
		items, err := svc.List(ctx, in.Limit, in.Offset)
		if err != nil {
			return nil, err
		}
		return &deploymentListOutput{Body: items}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:  "merge-approved-merge-request",
		Method:       http.MethodPost,
		Path:         "/api/admin/merge-requests/{iid}/merge",
		Summary:      "Approve and merge a verified merge request",
		Tags:         []string{"Deployments"},
		MaxBodyBytes: 4096,
	}, func(ctx context.Context, in *mergeRequestInput) (*mergeRequestOutput, error) {
		result, err := svc.Merge(ctx, in.IID, in.Body.SHA)
		if err != nil {
			return nil, err
		}
		return &mergeRequestOutput{Body: result}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:  "run-deployment-operation",
		Method:       http.MethodPost,
		Path:         "/api/admin/deployments/{action}",
		Summary:      "Run a constrained deployment operation",
		Tags:         []string{"Deployments"},
		MaxBodyBytes: 4096,
	}, func(ctx context.Context, in *runDeploymentInput) (*runDeploymentOutput, error) {
		principal, err := identity.FromContext(ctx)
		if err != nil {
			return nil, err
		}
		result, err := svc.Run(ctx, principal, in.Action, in.Body.Target, in.Body.Release)
		if err != nil {
			return nil, err
		}
		return &runDeploymentOutput{Body: result}, nil
	})
}
