package codingtasks

import (
	"context"
	"testing"

	db "hackathon/internal/db/sqlc"
	"hackathon/internal/deployments"
	"hackathon/internal/identity"
	"hackathon/internal/plannerblocks"
)

type recordingPlanner struct{ calls int }

func (p *recordingPlanner) CreateForAI(_ context.Context, _ identity.Principal, input plannerblocks.CreateInput) (plannerblocks.PlannerBlock, error) {
	p.calls++
	return plannerblocks.PlannerBlock{ID: 7, Title: input.Title, Owner: plannerblocks.Owner{Username: "niktar411"}}, nil
}

func (p *recordingPlanner) StartForOwner(_ context.Context, id int64, username string) (plannerblocks.PlannerBlock, error) {
	p.calls++
	return plannerblocks.PlannerBlock{ID: id, Owner: plannerblocks.Owner{Username: username}, Progress: plannerblocks.Progress{State: plannerblocks.ProgressVerifying}}, nil
}

func (p *recordingPlanner) ListOwned(_ context.Context, username string) ([]plannerblocks.PlannerBlock, error) {
	return []plannerblocks.PlannerBlock{{ID: 7, Title: "Real block", Owner: plannerblocks.Owner{Username: username}}}, nil
}

type recordingDeployments struct {
	calls  int
	action string
	target string
}

func (d *recordingDeployments) Run(_ context.Context, _ identity.Principal, action, target, _ string) (deployments.Deployment, error) {
	d.calls++
	d.action = action
	d.target = target
	return deployments.Deployment{ID: 9, Action: action, Target: target, Status: "completed"}, nil
}

func (d *recordingDeployments) Merge(_ context.Context, iid int64, sha string) (deployments.MergeResult, error) {
	d.calls++
	d.action = "merge"
	return deployments.MergeResult{IID: iid, SHA: sha, State: "merged", Merged: true}, nil
}

func confirmedAdminContext(ctx context.Context) context.Context {
	return identity.WithPrincipal(ctx, identity.Principal{
		AuthKind: "local", IP: "10.77.0.2",
		Session: db.Session{Username: "niktar411"},
		Account: db.LocalAccount{Username: "niktar411", Role: "admin", Enabled: true},
	})
}

func TestAdminToolsRequireAdministratorAndFixedOperations(t *testing.T) {
	planner := &recordingPlanner{}
	deployment := &recordingDeployments{}
	service := &Service{config: Config{AdminUser: "niktar411", AdminIP: "10.77.0.2"}}
	service.SetAdminTools(planner, deployment)

	output, err := service.callAdminOperation(t.Context(), ToolOperateService, ToolInput{Action: "restart", Target: "production"})
	if err != nil || output.OK || planner.calls != 0 || deployment.calls != 0 {
		t.Fatalf("unauthenticated admin tool: %+v, %v", output, err)
	}
	ctx := confirmedAdminContext(t.Context())
	output, err = service.callAdminOperation(ctx, ToolOperateService, ToolInput{Action: "deploy", Target: "production"})
	if err != nil || output.OK || deployment.calls != 0 {
		t.Fatalf("direct AI deploy must be rejected: %+v, %v", output, err)
	}
	output, err = service.callAdminOperation(ctx, ToolOperateService, ToolInput{Action: "restart", Target: "production"})
	if err != nil || !output.OK || deployment.calls != 1 || deployment.action != "restart" {
		t.Fatalf("valid constrained operation failed: %+v, %v", output, err)
	}
	output, err = service.callAdminOperation(ctx, ToolCreatePlannerBlock, ToolInput{Title: "Real block", OwnerID: 1})
	if err != nil || !output.OK || planner.calls != 1 {
		t.Fatalf("valid planner operation failed: %+v, %v", output, err)
	}
}
