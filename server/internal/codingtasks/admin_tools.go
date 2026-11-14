package codingtasks

import (
	"context"
	"encoding/json"
	"fmt"

	"hackathon/internal/deployments"
	"hackathon/internal/identity"
	"hackathon/internal/plannerblocks"
)

const (
	ToolCreatePlannerBlock = "create_planner_block"
	ToolOperateService     = "operate_service"
)

type AdminPlanner interface {
	CreateForAI(ctx context.Context, actor identity.Principal, input plannerblocks.CreateInput) (plannerblocks.PlannerBlock, error)
}

type AdminDeployments interface {
	Run(ctx context.Context, principal identity.Principal, action, target, release string) (deployments.Deployment, error)
}

func (s *Service) SetAdminTools(planner AdminPlanner, deployment AdminDeployments) {
	s.adminPlanner = planner
	s.adminDeployments = deployment
}

func isAdminOperation(operation string) bool {
	return operation == ToolCreatePlannerBlock || operation == ToolOperateService
}

func (s *Service) callAdminOperation(ctx context.Context, operation string, input ToolInput) (ToolOutput, error) {
	principal, err := identity.FromContext(ctx)
	if err != nil || !identity.IsAdmin(principal, s.config.AdminUser, s.config.AdminIP) {
		return ToolOutput{OK: false, Error: "admin_access_required"}, nil
	}
	var result any
	switch operation {
	case ToolCreatePlannerBlock:
		if s.adminPlanner == nil || input.OwnerID < 1 || input.Title == "" {
			return ToolOutput{OK: false, Error: "invalid_planner_operation"}, nil
		}
		result, err = s.adminPlanner.CreateForAI(ctx, principal, plannerblocks.CreateInput{
			Title: input.Title, Description: input.Description, OwnerID: input.OwnerID,
			Position: input.Position, Branch: input.Branch, MRIID: input.MRIID,
		})
	case ToolOperateService:
		if s.adminDeployments == nil || !validAIOperation(input.Action, input.Target) {
			return ToolOutput{OK: false, Error: "invalid_service_operation"}, nil
		}
		result, err = s.adminDeployments.Run(ctx, principal, input.Action, input.Target, "")
	default:
		return ToolOutput{OK: false, Error: "invalid_admin_operation"}, nil
	}
	if err != nil {
		return ToolOutput{OK: false, Error: "admin_operation_failed"}, nil
	}
	data, err := json.Marshal(result)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("encode admin tool result: %w", err)
	}
	return ToolOutput{OK: true, Content: string(data)}, nil
}

func validAIOperation(action, target string) bool {
	switch action {
	case deployments.ActionStart, deployments.ActionStop, deployments.ActionRestart,
		deployments.ActionCreateTestDatabase, deployments.ActionDropTestDatabase:
	default:
		return false
	}
	switch target {
	case deployments.TargetProduction, deployments.TargetCoding, deployments.TargetReview, deployments.TargetTest:
		return true
	}
	return false
}
