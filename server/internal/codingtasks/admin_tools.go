package codingtasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hackathon/internal/deployments"
	"hackathon/internal/identity"
	"hackathon/internal/plannerblocks"
)

const (
	ToolCreatePlannerBlock = "create_planner_block"
	ToolStartPlannerBlock  = "start_planner_block"
	ToolOperateService     = "operate_service"
	ToolMergeMergeRequest  = "merge_merge_request"
)

type AdminPlanner interface {
	CreateForAI(ctx context.Context, actor identity.Principal, input plannerblocks.CreateInput) (plannerblocks.PlannerBlock, error)
	StartForOwner(ctx context.Context, id int64, username string) (plannerblocks.PlannerBlock, error)
	ListOwned(ctx context.Context, username string) ([]plannerblocks.PlannerBlock, error)
}

type AdminDeployments interface {
	Run(ctx context.Context, principal identity.Principal, action, target, release string) (deployments.Deployment, error)
	Merge(ctx context.Context, iid int64, sha string) (deployments.MergeResult, error)
}

func (s *Service) SetAdminTools(planner AdminPlanner, deployment AdminDeployments) {
	s.adminPlanner = planner
	s.adminDeployments = deployment
}

func isAdminOperation(operation string) bool {
	return operation == ToolCreatePlannerBlock || operation == ToolOperateService || operation == ToolMergeMergeRequest
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
	case ToolMergeMergeRequest:
		if s.adminDeployments == nil || input.MRIID == nil || *input.MRIID < 1 {
			return ToolOutput{OK: false, Error: "invalid_merge_request"}, nil
		}
		sha := strings.TrimSpace(input.SHA)
		if sha == "" && s.gitLab != nil {
			mergeRequest, lookupErr := s.gitLab.MergeRequestByIID(ctx, *input.MRIID)
			if lookupErr != nil {
				return ToolOutput{OK: false, Error: "merge_request_unavailable"}, nil
			}
			sha = mergeRequest.SHA
		}
		result, err = s.adminDeployments.Merge(ctx, *input.MRIID, sha)
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
