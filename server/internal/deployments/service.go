package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
	"hackathon/internal/identity"
)

const (
	operationTimeout = 2 * time.Minute
	finishTimeout    = 5 * time.Second
)

// Store is implemented by *db.Queries using the generated deployments.sql
// methods. Keeping it behind an interface also makes authorization failures
// testable without a database.
type Store interface {
	CreateMergeApproval(ctx context.Context, arg db.CreateMergeApprovalParams) error
	GetMergeApproval(ctx context.Context, arg db.GetMergeApprovalParams) (db.MergeApproval, error)
	BeginDeploymentOperation(ctx context.Context, arg db.BeginDeploymentOperationParams) (db.DeploymentOperation, error)
	FinishDeploymentOperation(ctx context.Context, arg db.FinishDeploymentOperationParams) error
	ListDeploymentOperations(ctx context.Context, arg db.ListDeploymentOperationsParams) ([]db.DeploymentOperation, error)
	LatestSuccessfulRelease(ctx context.Context) (db.DeploymentOperation, error)
}

// GitLab is implemented by *gitlab.Client.
type GitLab interface {
	MergeRequestByIID(ctx context.Context, iid int64) (gitlab.MergeRequest, error)
	ApproveMergeRequest(ctx context.Context, iid int64, sha string) error
	Merge(ctx context.Context, iid int64, sha string) error
}

// Service owns merge approval and serialized deployment operations.
type Service struct {
	store         Store
	gitLab        GitLab
	orchestrator  Orchestrator
	config        Config
	operationLock chan struct{}
}

func NewService(store Store, gitLab GitLab, orchestrator Orchestrator, config Config) *Service {
	return &Service{
		store:         store,
		gitLab:        gitLab,
		orchestrator:  orchestrator,
		config:        config,
		operationLock: make(chan struct{}, 1),
	}
}

func (s *Service) requireAdmin(ctx context.Context) (identity.Principal, error) {
	if s.config.AdminUser == "" || s.config.AdminIP == "" {
		return identity.Principal{}, huma.Error503ServiceUnavailable("auth.admin_not_configured")
	}
	return identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP)
}

func (s *Service) requirePrincipal(ctx context.Context, principal identity.Principal) (identity.Principal, error) {
	return s.requireAdmin(identity.WithPrincipal(ctx, principal))
}

func (s *Service) List(ctx context.Context, limit, offset int32) ([]Deployment, error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, huma.Error400BadRequest("deployment.invalid_pagination")
	}
	rows, err := s.store.ListDeploymentOperations(ctx, db.ListDeploymentOperationsParams{Limit: limit, Offset: offset})
	if err != nil {
		slog.ErrorContext(ctx, "list deployment operations failed", "error", err)
		return nil, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	result := make([]Deployment, 0, len(rows))
	for _, row := range rows {
		result = append(result, deploymentFromRow(row))
	}
	return result, nil
}

// Merge reloads the merge request before making any decision. The caller's SHA
// is a concurrency precondition and is never used as a substitute for GitLab's
// current SHA.
func (s *Service) Merge(ctx context.Context, iid int64, expectedSHA string) (MergeResult, error) {
	principal, err := s.requireAdmin(ctx)
	if err != nil {
		return MergeResult{}, err
	}
	if iid < 1 {
		return MergeResult{}, huma.Error400BadRequest("deployment.invalid_merge_request")
	}
	expectedSHA = strings.TrimSpace(expectedSHA)
	if expectedSHA == "" || !validRelease(expectedSHA) {
		return MergeResult{}, huma.Error400BadRequest("deployment.invalid_sha")
	}
	mr, err := s.gitLab.MergeRequestByIID(ctx, iid)
	if err != nil {
		slog.ErrorContext(ctx, "reload merge request failed", "iid", iid, "error", err)
		return MergeResult{}, huma.Error502BadGateway("gitlab.merge_request_unavailable")
	}
	if mr.TargetBranch != "main" {
		return MergeResult{}, huma.Error409Conflict("deployment.merge_target_invalid")
	}
	if mr.State != "opened" {
		return MergeResult{}, huma.Error409Conflict("deployment.merge_request_not_open")
	}
	if mr.SHA == "" || mr.SHA != expectedSHA {
		return MergeResult{}, huma.Error409Conflict("deployment.merge_sha_mismatch")
	}
	if !pipelineSuccessful(mr) {
		return MergeResult{}, huma.Error409Conflict("deployment.pipeline_not_successful")
	}
	if err := s.store.CreateMergeApproval(ctx, db.CreateMergeApprovalParams{
		MergeRequestIid: iid,
		SourceBranch:    mr.SourceBranch,
		TargetBranch:    mr.TargetBranch,
		CommitSha:       mr.SHA,
		ApprovedBy:      principal.Session.Username,
	}); err != nil {
		slog.ErrorContext(ctx, "persist merge approval failed", "iid", iid, "error", err)
		return MergeResult{}, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	if err := s.gitLab.ApproveMergeRequest(ctx, iid, mr.SHA); err != nil {
		slog.ErrorContext(ctx, "approve merge request failed", "iid", iid, "error", err)
		return MergeResult{}, huma.Error502BadGateway("gitlab.approval_failed")
	}
	if err := s.gitLab.Merge(ctx, iid, mr.SHA); err != nil {
		slog.ErrorContext(ctx, "merge request failed", "iid", iid, "error", err)
		return MergeResult{}, huma.Error502BadGateway("gitlab.merge_failed_check_state")
	}
	actual, err := s.gitLab.MergeRequestByIID(ctx, iid)
	if err != nil {
		slog.ErrorContext(ctx, "reload merged request failed", "iid", iid, "error", err)
		return MergeResult{}, huma.Error502BadGateway("gitlab.merge_result_unknown")
	}
	if actual.State != "merged" || actual.SHA != expectedSHA {
		return MergeResult{}, huma.Error502BadGateway("gitlab.merge_result_unconfirmed")
	}
	return MergeResult{
		IID:    actual.IID,
		State:  actual.State,
		SHA:    actual.SHA,
		WebURL: actual.WebURL,
		Merged: actual.State == "merged",
	}, nil
}

// Run is also the entry point for AI tools. Authorization is checked here, not
// delegated to a router or model prompt.
func (s *Service) Run(ctx context.Context, principal identity.Principal, action, target, release string) (Deployment, error) {
	admin, err := s.requirePrincipal(ctx, principal)
	if err != nil {
		return Deployment{}, err
	}
	if action == ActionDeploy && target == TargetProduction {
		return Deployment{}, huma.Error409Conflict("deployment.verified_merge_required")
	}
	return s.execute(ctx, admin.Session.Username, action, target, release)
}

// execute is called by Run after admin authorization and by the webhook after
// its independent GitLab and persisted-approval checks.
func (s *Service) execute(ctx context.Context, actor, action, target, release string) (Deployment, error) {
	if !validAction(action) || !validTarget(target) {
		return Deployment{}, huma.Error400BadRequest("deployment.invalid_operation")
	}
	release = strings.TrimSpace(release)
	if !validRelease(release) || action == ActionDeploy && release == "" {
		return Deployment{}, huma.Error400BadRequest("deployment.invalid_release")
	}

	runCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	select {
	case s.operationLock <- struct{}{}:
		defer func() { <-s.operationLock }()
	case <-runCtx.Done():
		return Deployment{}, huma.Error503ServiceUnavailable("deployment.queue_unavailable")
	}

	operation, err := s.store.BeginDeploymentOperation(runCtx, db.BeginDeploymentOperationParams{
		Actor:   actor,
		Action:  action,
		Target:  target,
		Release: release,
	})
	if err != nil {
		slog.ErrorContext(runCtx, "begin deployment operation failed", "error", err)
		return Deployment{}, huma.Error503ServiceUnavailable("storage.unavailable")
	}

	agentResult, callErr := s.orchestrator.Run(runCtx, action, target, release)
	agentResult.BackupName = strings.TrimSpace(agentResult.BackupName)
	if !validBackupName(agentResult.BackupName) {
		agentResult.BackupName = ""
	}
	status, errorCode := classifyOperation(action, target, agentResult, callErr)
	if callErr != nil {
		slog.ErrorContext(runCtx, "deployment agent operation failed", "operation_id", operation.ID, "error", callErr)
	}
	details := operationDetails(agentResult.Status, errorCode)

	// A disconnected caller or an expired agent context must not prevent the
	// terminal state from being recorded.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(runCtx), finishTimeout)
	defer finishCancel()
	if err := s.store.FinishDeploymentOperation(finishCtx, db.FinishDeploymentOperationParams{
		ID:         operation.ID,
		Status:     status,
		BackupName: agentResult.BackupName,
		Details:    details,
	}); err != nil {
		slog.ErrorContext(ctx, "finish deployment operation failed", "operation_id", operation.ID, "error", err)
		return Deployment{}, huma.Error503ServiceUnavailable("deployment.unknown_outcome")
	}

	finishedAt := time.Now().UTC()
	operation.Status = status
	operation.BackupName = agentResult.BackupName
	operation.Details = details
	operation.FinishedAt = pgtype.Timestamptz{Time: finishedAt, Valid: true}
	result := deploymentFromRow(operation)
	if errorCode != "" {
		return result, huma.Error502BadGateway(errorCode)
	}
	return result, nil
}

func classifyOperation(action, target string, result AgentResult, callErr error) (status, errorCode string) {
	agentStatus := strings.ToLower(strings.TrimSpace(result.Status))
	if callErr == nil && agentStatus == "completed" {
		if productionBackupRequired(action, target) && result.BackupName == "" {
			return "failed", "deployment.backup_unconfirmed"
		}
		return "completed", ""
	}
	if agentStatus == "rolled_back" && action != ActionRollback {
		return "rolled_back", "deployment.rolled_back"
	}
	if agentStatus == "rollback_failed" || action == ActionRollback {
		return "failed", "deployment.rollback_failed_check_state"
	}
	if errors.Is(callErr, context.DeadlineExceeded) {
		return "failed", "deployment.timeout_check_state"
	}
	if callErr == nil {
		return "failed", "deployment.unknown_outcome"
	}
	return "failed", "deployment.failed_check_state"
}

func productionBackupRequired(action, target string) bool {
	if target != TargetProduction {
		return false
	}
	return action == ActionDeploy || action == ActionRestart || action == ActionRollback
}

func operationDetails(agentStatus, errorCode string) []byte {
	switch strings.ToLower(strings.TrimSpace(agentStatus)) {
	case "completed", "failed", "rolled_back", "rollback_failed":
		agentStatus = strings.ToLower(strings.TrimSpace(agentStatus))
	default:
		agentStatus = "unknown"
	}
	details, _ := json.Marshal(struct {
		AgentStatus string `json:"agentStatus"`
		Error       string `json:"error,omitempty"`
	}{AgentStatus: agentStatus, Error: errorCode})
	return details
}

func deploymentFromRow(row db.DeploymentOperation) Deployment {
	result := Deployment{
		ID:         row.ID,
		Actor:      row.Actor,
		Action:     row.Action,
		Target:     row.Target,
		Release:    row.Release,
		Status:     row.Status,
		BackupName: row.BackupName,
		Details:    json.RawMessage("{}"),
	}
	if json.Valid(row.Details) {
		result.Details = append(json.RawMessage(nil), row.Details...)
	}
	if row.CreatedAt.Valid {
		result.CreatedAt = row.CreatedAt.Time
	}
	if row.FinishedAt.Valid {
		finishedAt := row.FinishedAt.Time
		result.FinishedAt = &finishedAt
	}
	return result
}

func pipelineSuccessful(mr gitlab.MergeRequest) bool {
	return mr.HeadPipeline.Status == "success" &&
		(mr.HeadPipeline.SHA == "" || mr.HeadPipeline.SHA == mr.SHA)
}

func validAction(action string) bool {
	_, ok := agentPaths[action]
	return ok
}

func validTarget(target string) bool {
	switch target {
	case TargetProduction, TargetCoding, TargetReview, TargetTest:
		return true
	default:
		return false
	}
}

func validRelease(release string) bool {
	if release == "" {
		return true
	}
	if len(release) > 128 || !alphaNumeric(release[0]) {
		return false
	}
	for i := 0; i < len(release); i++ {
		if !alphaNumeric(release[i]) && release[i] != '_' && release[i] != '-' && release[i] != '.' {
			return false
		}
	}
	return true
}

func alphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func validBackupName(backupName string) bool {
	return printableWithin(backupName, 255)
}

func printableWithin(value string, maximum int) bool {
	if len(value) > maximum {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
