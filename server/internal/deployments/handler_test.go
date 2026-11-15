package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
	"hackathon/internal/identity"
)

const testSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type fakeStore struct {
	approvals   map[string]db.MergeApproval
	created     []db.CreateMergeApprovalParams
	operations  []db.DeploymentOperation
	finished    []db.FinishDeploymentOperationParams
	latest      db.DeploymentOperation
	latestErr   error
	listErr     error
	beginErr    error
	finishErr   error
	approvalErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{approvals: map[string]db.MergeApproval{}, latestErr: pgx.ErrNoRows}
}

func approvalKey(iid int64, sha string) string {
	return strconv.FormatInt(iid, 10) + ":" + sha
}

func (s *fakeStore) CreateMergeApproval(_ context.Context, arg db.CreateMergeApprovalParams) error {
	if s.approvalErr != nil {
		return s.approvalErr
	}
	s.created = append(s.created, arg)
	key := approvalKey(arg.MergeRequestIid, arg.CommitSha)
	if _, exists := s.approvals[key]; !exists {
		s.approvals[key] = db.MergeApproval{
			ID:              int64(len(s.approvals) + 1),
			MergeRequestIid: arg.MergeRequestIid,
			SourceBranch:    arg.SourceBranch,
			TargetBranch:    arg.TargetBranch,
			CommitSha:       arg.CommitSha,
			ApprovedBy:      arg.ApprovedBy,
		}
	}
	return nil
}

func (s *fakeStore) GetMergeApproval(_ context.Context, arg db.GetMergeApprovalParams) (db.MergeApproval, error) {
	approval, ok := s.approvals[approvalKey(arg.MergeRequestIid, arg.CommitSha)]
	if !ok || approval.TargetBranch != "main" {
		return db.MergeApproval{}, pgx.ErrNoRows
	}
	return approval, nil
}

func (s *fakeStore) BeginDeploymentOperation(_ context.Context, arg db.BeginDeploymentOperationParams) (db.DeploymentOperation, error) {
	if s.beginErr != nil {
		return db.DeploymentOperation{}, s.beginErr
	}
	operation := db.DeploymentOperation{
		ID:        int64(len(s.operations) + 1),
		Actor:     arg.Actor,
		Action:    arg.Action,
		Target:    arg.Target,
		Release:   arg.Release,
		Status:    "running",
		Details:   []byte(`{}`),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}
	s.operations = append(s.operations, operation)
	return operation, nil
}

func (s *fakeStore) FinishDeploymentOperation(_ context.Context, arg db.FinishDeploymentOperationParams) error {
	if s.finishErr != nil {
		return s.finishErr
	}
	s.finished = append(s.finished, arg)
	for i := range s.operations {
		if s.operations[i].ID == arg.ID {
			s.operations[i].Status = arg.Status
			s.operations[i].BackupName = arg.BackupName
			s.operations[i].Details = append([]byte(nil), arg.Details...)
		}
	}
	return nil
}

func (s *fakeStore) ListDeploymentOperations(_ context.Context, _ db.ListDeploymentOperationsParams) ([]db.DeploymentOperation, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]db.DeploymentOperation(nil), s.operations...), nil
}

func (s *fakeStore) LatestSuccessfulRelease(_ context.Context) (db.DeploymentOperation, error) {
	return s.latest, s.latestErr
}

type fakeGitLab struct {
	mr           gitlab.MergeRequest
	afterMerge   *gitlab.MergeRequest
	loadErr      error
	approveErr   error
	mergeErr     error
	loads        int
	approveCalls int
	mergeCalls   int
	approvedSHA  string
	mergedSHA    string
}

func (g *fakeGitLab) MergeRequestByIID(_ context.Context, _ int64) (gitlab.MergeRequest, error) {
	g.loads++
	if g.loadErr != nil {
		return gitlab.MergeRequest{}, g.loadErr
	}
	if g.mergeCalls > 0 && g.afterMerge != nil {
		return *g.afterMerge, nil
	}
	return g.mr, nil
}

func (g *fakeGitLab) ApproveMergeRequest(_ context.Context, _ int64, sha string) error {
	g.approveCalls++
	g.approvedSHA = sha
	return g.approveErr
}

func (g *fakeGitLab) Merge(_ context.Context, _ int64, sha string) error {
	g.mergeCalls++
	g.mergedSHA = sha
	return g.mergeErr
}

type agentCall struct {
	action  string
	target  string
	release string
}

type fakeOrchestrator struct {
	mu     sync.Mutex
	calls  []agentCall
	result AgentResult
	err    error
}

func (o *fakeOrchestrator) Run(_ context.Context, action, target, release string) (AgentResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, agentCall{action: action, target: target, release: release})
	return o.result, o.err
}

func (o *fakeOrchestrator) callCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.calls)
}

type deploymentFixture struct {
	service      *Service
	store        *fakeStore
	gitLab       *fakeGitLab
	orchestrator *fakeOrchestrator
}

func newDeploymentFixture() *deploymentFixture {
	store := newFakeStore()
	gitLab := &fakeGitLab{}
	orchestrator := &fakeOrchestrator{}
	config := Config{
		AdminUser:           "admin",
		AdminIP:             "10.77.0.2",
		GitLabWebhookSecret: "webhook-secret",
	}
	return &deploymentFixture{
		service:      NewService(store, gitLab, orchestrator, config),
		store:        store,
		gitLab:       gitLab,
		orchestrator: orchestrator,
	}
}

func adminPrincipal() identity.Principal {
	return identity.Principal{
		AuthKind: "local",
		IP:       "10.77.0.2",
		Session:  db.Session{Username: "admin"},
		Account: db.LocalAccount{
			Username: "admin",
			Role:     "admin",
			Enabled:  true,
		},
	}
}

func adminContext() context.Context {
	return identity.WithPrincipal(context.Background(), adminPrincipal())
}

func openMergeRequest() gitlab.MergeRequest {
	return gitlab.MergeRequest{
		IID:          42,
		State:        "opened",
		SourceBranch: "feature",
		TargetBranch: "main",
		SHA:          testSHA,
		WebURL:       "https://gitlab.example/mr/42",
		HeadPipeline: gitlab.Pipeline{Status: "success", SHA: testSHA},
	}
}

func mergedMergeRequest() gitlab.MergeRequest {
	mr := openMergeRequest()
	mr.State = "merged"
	return mr
}

func requireStatus(t *testing.T, err error, expected int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected status %d, got nil error", expected)
	}
	var statusError interface{ GetStatus() int }
	if !errors.As(err, &statusError) {
		t.Fatalf("error %T does not expose an HTTP status: %v", err, err)
	}
	if statusError.GetStatus() != expected {
		t.Fatalf("status = %d, want %d (error %v)", statusError.GetStatus(), expected, err)
	}
}

func TestRegisterBuildsAdministrativeRoutes(t *testing.T) {
	_, api := humatest.New(t)
	Register(api, nil)
	for _, path := range []string{
		"/api/admin/deployments",
		"/api/admin/deployments/{action}",
		"/api/admin/merge-requests/{iid}/merge",
	} {
		if api.OpenAPI().Paths[path] == nil {
			t.Errorf("OpenAPI path %s is not registered", path)
		}
	}
}

func TestRunRejectsNonAdminBeforeSideEffects(t *testing.T) {
	fixture := newDeploymentFixture()
	principal := adminPrincipal()
	principal.Account.Role = "user"

	_, err := fixture.service.Run(context.Background(), principal, ActionDeploy, TargetProduction, testSHA)
	requireStatus(t, err, http.StatusForbidden)
	if fixture.orchestrator.callCount() != 0 || len(fixture.store.operations) != 0 {
		t.Fatal("non-admin operation reached the database or agent")
	}
}

func TestDirectProductionDeployRequiresVerifiedMerge(t *testing.T) {
	fixture := newDeploymentFixture()
	_, err := fixture.service.Run(context.Background(), adminPrincipal(), ActionDeploy, TargetProduction, testSHA)
	requireStatus(t, err, http.StatusConflict)
	if fixture.orchestrator.callCount() != 0 || len(fixture.store.operations) != 0 {
		t.Fatal("direct production deploy bypassed the merge webhook")
	}
}

func TestReleaseCannotContainPathTraversal(t *testing.T) {
	fixture := newDeploymentFixture()
	_, err := fixture.service.Run(context.Background(), adminPrincipal(), ActionRollback, TargetProduction, "../secret")
	requireStatus(t, err, http.StatusBadRequest)
	if fixture.orchestrator.callCount() != 0 || len(fixture.store.operations) != 0 {
		t.Fatal("unsafe release reached side effects")
	}
}

func TestMergeRejectsRedPipelineBeforeApproval(t *testing.T) {
	fixture := newDeploymentFixture()
	fixture.gitLab.mr = openMergeRequest()
	fixture.gitLab.mr.HeadPipeline.Status = "failed"

	_, err := fixture.service.Merge(adminContext(), 42, testSHA)
	requireStatus(t, err, http.StatusConflict)
	if len(fixture.store.created) != 0 || fixture.gitLab.approveCalls != 0 || fixture.gitLab.mergeCalls != 0 {
		t.Fatal("red pipeline was approved or merged")
	}
}

func TestMergeReturnsReloadedGitLabResult(t *testing.T) {
	fixture := newDeploymentFixture()
	fixture.gitLab.mr = openMergeRequest()
	actual := mergedMergeRequest()
	actual.WebURL = "https://gitlab.example/mr/42/merged"
	fixture.gitLab.afterMerge = &actual

	result, err := fixture.service.Merge(adminContext(), 42, testSHA)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if !result.Merged || result.State != "merged" || result.WebURL != actual.WebURL {
		t.Fatalf("result is not the reloaded merge request: %+v", result)
	}
	if len(fixture.store.created) != 1 {
		t.Fatalf("persisted approvals = %d, want 1", len(fixture.store.created))
	}
	approval := fixture.store.created[0]
	if approval.CommitSha != testSHA || approval.ApprovedBy != "admin" || approval.TargetBranch != "main" {
		t.Fatalf("unexpected approval: %+v", approval)
	}
	if fixture.gitLab.approvedSHA != testSHA || fixture.gitLab.mergedSHA != testSHA {
		t.Fatal("GitLab approval/merge did not use the verified SHA")
	}
}

func webhookRequest(token string) *http.Request {
	body := `{"object_kind":"merge_request","object_attributes":{"iid":42,"sha":"untrusted-payload-sha"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/gitlab", strings.NewReader(body))
	req.Header.Set("X-Gitlab-Token", token)
	return req
}

func TestWebhookRejectsMissingExactApproval(t *testing.T) {
	fixture := newDeploymentFixture()
	fixture.gitLab.mr = mergedMergeRequest()
	response := httptest.NewRecorder()

	fixture.service.HandleGitLabWebhook(response, webhookRequest("webhook-secret"))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fixture.orchestrator.callCount() != 0 {
		t.Fatal("webhook deployed without an exact-SHA approval")
	}
}

func TestWebhookRejectsForgedTokenBeforeGitLab(t *testing.T) {
	fixture := newDeploymentFixture()
	fixture.gitLab.mr = mergedMergeRequest()
	response := httptest.NewRecorder()

	fixture.service.HandleGitLabWebhook(response, webhookRequest("forged"))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fixture.gitLab.loads != 0 || fixture.orchestrator.callCount() != 0 {
		t.Fatal("forged webhook reached GitLab or the deployment agent")
	}
}

func TestWebhookDeploysVerifiedRelease(t *testing.T) {
	fixture := newDeploymentFixture()
	fixture.gitLab.mr = mergedMergeRequest()
	fixture.store.approvals[approvalKey(42, testSHA)] = db.MergeApproval{
		ID:              1,
		MergeRequestIid: 42,
		SourceBranch:    "feature",
		TargetBranch:    "main",
		CommitSha:       testSHA,
		ApprovedBy:      "admin",
	}
	fixture.orchestrator.result = AgentResult{Status: "completed", BackupName: "pre-deploy-backup"}
	response := httptest.NewRecorder()

	fixture.service.HandleGitLabWebhook(response, webhookRequest("webhook-secret"))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fixture.orchestrator.callCount() != 1 {
		t.Fatalf("agent calls = %d, want 1", fixture.orchestrator.callCount())
	}
	call := fixture.orchestrator.calls[0]
	if call.action != ActionDeploy || call.target != TargetProduction || call.release != testSHA {
		t.Fatalf("unexpected deployment: %+v", call)
	}
	if len(fixture.store.finished) != 1 || fixture.store.finished[0].Status != "completed" ||
		fixture.store.finished[0].BackupName != "pre-deploy-backup" {
		t.Fatalf("deployment result was not persisted: %+v", fixture.store.finished)
	}
	var body Deployment
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Release != testSHA || body.Status != "completed" {
		t.Fatalf("unexpected webhook response: %+v", body)
	}
}

func TestRollbackFailureHasSafeMappingAndAudit(t *testing.T) {
	fixture := newDeploymentFixture()
	fixture.orchestrator.result = AgentResult{
		Status:     "failed",
		BackupName: "pre-rollback-backup",
		Details:    json.RawMessage(`{"error":"secret command output"}`),
	}
	fixture.orchestrator.err = errors.New("rollback failed: secret-token")

	_, err := fixture.service.Run(context.Background(), adminPrincipal(), ActionRollback, TargetProduction, "previous-release")
	requireStatus(t, err, http.StatusBadGateway)
	if err.Error() != "deployment.rollback_failed_check_state" {
		t.Fatalf("error = %q", err)
	}
	if len(fixture.store.finished) != 1 || fixture.store.finished[0].Status != "failed" {
		t.Fatalf("rollback failure was not persisted: %+v", fixture.store.finished)
	}
	details := string(fixture.store.finished[0].Details)
	if strings.Contains(details, "secret") || !strings.Contains(details, "deployment.rollback_failed_check_state") {
		t.Fatalf("unsafe or incomplete audit details: %s", details)
	}
}

func TestAgentOrchestratorUsesFixedAuthenticatedEndpoint(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/deployments/deploy" || r.Method != http.MethodPost {
			t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer agent-secret" {
			t.Errorf("missing agent authentication")
		}
		var body struct {
			Target  string `json:"target"`
			Release string `json:"release"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Target != TargetProduction || body.Release != testSHA {
			t.Errorf("unexpected request: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","backup_name":"backup-1"}`))
	}))
	defer server.Close()

	client := NewOrchestrator(Config{AgentURL: server.URL, AgentToken: "agent-secret"}, server.Client())
	result, err := client.Run(context.Background(), ActionDeploy, TargetProduction, testSHA)
	if err != nil {
		t.Fatalf("agent request failed: %v", err)
	}
	if result.Status != "completed" || result.BackupName != "backup-1" {
		t.Fatalf("unexpected agent result: %+v", result)
	}
	if _, err := client.Run(context.Background(), "exec", TargetProduction, testSHA); err == nil {
		t.Fatal("arbitrary operation was accepted")
	}
	if requests != 1 {
		t.Fatalf("HTTP requests = %d, want 1", requests)
	}
}
