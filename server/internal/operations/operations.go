package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"hackathon/internal/codingtasks"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/deployments"
	"hackathon/internal/gitlab"
	"hackathon/internal/infra/metrics"
	"hackathon/internal/llm"
	"hackathon/internal/localaccounts"
	"hackathon/internal/notifications"
	"hackathon/internal/plannerblocks"
	"hackathon/internal/questions"
	"hackathon/internal/sam"
	"hackathon/internal/sam/assistant"
	"hackathon/internal/sam/compliance"
	"hackathon/internal/sam/guard"
	"hackathon/internal/sam/matching"
	"hackathon/internal/sam/operator"
)

type Service struct {
	db            *db.Queries
	pool          *pgxpool.Pool
	config        Config
	http          *http.Client
	modelHTTP     *http.Client
	model         *llm.Service
	chatSlots     chan struct{}
	mu            sync.Mutex
	activeUsers   map[string]bool
	accounts      *localaccounts.Service
	notifications *notifications.Service
	questions     *questions.Service
	planner       *plannerblocks.Service
	sam           *sam.Service
	samAssistant  *assistant.Service
	samMatching   *matching.Service
	samCompliance *compliance.Service
	samOperator   *operator.Service
	coding        *codingtasks.Service
	deployments   *deployments.Service
	gitLab        *gitlab.Client
	projectMu     sync.Mutex
	projectAt     time.Time
	projectText   string
}

func NewService(pool *pgxpool.Pool, c Config) *Service {
	modelHTTP := &http.Client{Timeout: 120 * time.Second, CheckRedirect: noRedirect}
	s := &Service{db: db.New(pool), pool: pool, config: c, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: noRedirect}, modelHTTP: modelHTTP, model: llm.NewService(pool, llm.Config{URL: c.ModelURL, APIKey: c.ModelKey, Model: c.Model, EncryptionKey: c.ModelEncryptionKey}, modelHTTP), chatSlots: make(chan struct{}, 2), activeUsers: map[string]bool{}}
	gitLab := gitlab.NewClient(gitlab.Config{URL: c.GitLabURL, Token: c.GitLabToken, Project: c.GitLabProject, Provider: c.Provider}, s.http)
	s.gitLab = gitLab
	s.accounts = localaccounts.NewService(pool, localaccounts.Config{PublicURL: c.PublicURL, PlannerURL: c.PlannerURL, AIURL: c.AIURL, AdminUser: c.AdminUser, AdminIP: c.AdminIP, SecureCookies: strings.HasPrefix(c.PublicURL, "https://")})
	s.notifications = notifications.NewService(pool)
	s.questions = questions.NewService(pool, s.model, questions.Config{PublicBaseURL: c.PublicURL, PlannerBaseURL: c.PlannerURL, AdminUser: c.AdminUser, AdminIP: c.AdminIP})
	s.planner = plannerblocks.NewService(pool, gitLab, plannerblocks.Config{AdminUser: c.AdminUser, AdminIP: c.AdminIP})
	s.planner.SetNotifier(s.notifications)
	modelGuard := guard.NewLimiter(2, 1)
	s.sam = sam.NewService(pool)
	s.samAssistant = assistant.NewService(pool, s.model, modelGuard)
	s.samMatching = matching.NewService(pool)
	s.samCompliance = compliance.NewService(pool)
	s.samOperator = operator.NewService(pool, s.model, modelGuard, operator.Config{AdminUser: c.AdminUser, AdminIP: c.AdminIP})
	httpWorker, err := codingtasks.NewHTTPWorker(c.WorkerURL, c.WorkerToken, s.http)
	var worker codingtasks.Worker
	if err == nil {
		worker = httpWorker
	}
	s.coding = codingtasks.NewService(s.db, s.model, worker, gitLab, codingtasks.Config{AdminUser: c.AdminUser, AdminIP: c.AdminIP, WorkerURL: c.WorkerURL, WorkerToken: c.WorkerToken, AsyncExecution: true})
	s.coding.SetNotifier(s.notifications)
	orchestrator := deployments.Orchestrator(deployments.NewOrchestrator(deployments.Config{AgentURL: c.AgentURL, AgentToken: c.AgentToken}, s.http))
	if c.WorkerAgentURL != "" && c.WorkerAgentToken != "" {
		workerOrchestrator := deployments.NewOrchestrator(deployments.Config{AgentURL: c.WorkerAgentURL, AgentToken: c.WorkerAgentToken}, s.http)
		orchestrator = deployments.NewTargetOrchestrator(orchestrator, workerOrchestrator)
	}
	s.deployments = deployments.NewService(s.db, gitLab, orchestrator, deployments.Config{AdminUser: c.AdminUser, AdminIP: c.AdminIP, AgentURL: c.AgentURL, AgentToken: c.AgentToken, GitLabWebhookSecret: c.GitLabWebhookSecret})
	s.coding.SetAdminTools(s.planner, s.deployments)
	return s
}

func noRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

// RecoverCodingTasks fails executions left running by a previous process.
func (s *Service) RecoverCodingTasks(ctx context.Context) error {
	if s.coding == nil {
		return nil
	}
	return s.coding.Recover(ctx)
}

func (s *Service) agent(ctx context.Context, path string, body interface{}, out interface{}) error {
	return s.controlRequest(ctx, s.config.AgentURL, s.config.AgentToken, path, body, out)
}

func (s *Service) worker(ctx context.Context, path string, body interface{}, out interface{}) error {
	return s.controlRequest(ctx, s.config.WorkerURL, s.config.WorkerToken, path, body, out)
}

func (s *Service) controlRequest(ctx context.Context, baseURL, token, path string, body interface{}, out interface{}) error {
	var data []byte
	var err error
	method := "GET"
	if body != nil {
		method = "POST"
		data, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode agent request: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create agent request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("agent request: %w", err)
	}
	defer closeBody(ctx, res.Body)
	if res.StatusCode != 200 {
		return fmt.Errorf("agent status %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 131072)).Decode(out); err != nil {
		return fmt.Errorf("decode agent: %w", err)
	}
	return nil
}

func localMetrics(_ context.Context) Metrics {
	result := Metrics{Name: "Сервер 2 · приложения", SampledAt: time.Now().UTC()}
	host, err := metrics.HostMetrics("Сервер 2 · приложения", nil)
	if err != nil {
		result.Error = "metrics.unavailable"
		return result
	}
	result.Available = true
	result.CPUPercent = host.CPUPercent
	result.MemoryUsed = host.MemoryUsed
	result.MemoryTotal = host.MemoryTotal
	result.DiskUsed = host.DiskUsed
	result.DiskTotal = host.DiskTotal
	result.UptimeSeconds = host.UptimeSeconds
	return result
}

func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	if _, err := s.admin(ctx); err != nil {
		return Dashboard{}, err
	}
	result := Dashboard{Servers: []Metrics{}, Peers: []Peer{}, Totals: []UsageTotal{}, Audit: []AuditEvent{}, Containers: []ContainerMetrics{}}
	first := Metrics{Name: "Сервер 1 · сайт, GitLab и оркестратор", Error: "metrics.unavailable"}
	if err := s.agent(ctx, "/metrics", nil, &first); err != nil {
		slog.WarnContext(ctx, "server one agent metrics unavailable")
		first = localMetrics(ctx)
		first.Name = "Сервер 1 · сайт, GitLab и оркестратор"
	}
	second := Metrics{Name: "Сервер 2 · coding, review и test", Error: "metrics.unavailable"}
	if err := s.worker(ctx, "/metrics", nil, &second); err != nil {
		slog.WarnContext(ctx, "server two metrics unavailable")
		second.Available = false
	}
	result.Servers = append(result.Servers, first, second)
	result.Containers = append(result.Containers, first.Containers...)
	result.Containers = append(result.Containers, second.Containers...)
	if err := s.agent(ctx, "/peers", nil, &result.Peers); err != nil {
		result.PeerError = "peers.unavailable"
	}
	totals, err := s.db.UsageTotals(ctx)
	if err != nil {
		return result, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	for _, row := range totals {
		result.Totals = append(result.Totals, UsageTotal{Username: row.Username, Requests: row.Requests, UnknownRequests: row.UnknownRequests, InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, TotalTokens: row.TotalTokens})
	}
	audit, err := s.db.ListAudit(ctx)
	if err != nil {
		return result, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	for _, row := range audit {
		result.Audit = append(result.Audit, AuditEvent{ID: row.ID, Username: row.Username, Action: row.Action, Target: row.Target, Status: row.Status, CreatedAt: row.CreatedAt.Time})
	}
	return result, nil
}
func nullableInt(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}
func (s *Service) Usage(ctx context.Context, offset int32) ([]UsageEvent, error) {
	if _, err := s.admin(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.ListUsage(ctx, db.ListUsageParams{Limit: 100, Offset: offset})
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	result := make([]UsageEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, UsageEvent{ID: row.ID, Username: row.Username, Model: row.Model, Status: row.Status, InputTokens: nullableInt(row.InputTokens), OutputTokens: nullableInt(row.OutputTokens), TotalTokens: nullableInt(row.TotalTokens), CreatedAt: row.CreatedAt.Time})
	}
	return result, nil
}
func (s *Service) ChangePeer(ctx context.Context, id, action string) (PeerResult, error) {
	p, err := s.admin(ctx)
	if err != nil {
		return PeerResult{}, err
	}
	if (id != "egor" && id != "leonid" && id != "igor") || (action != "enable" && action != "disable" && action != "rotate") {
		return PeerResult{}, huma.Error400BadRequest("peer.invalid")
	}
	auditID, err := s.db.BeginAudit(ctx, db.BeginAuditParams{Username: p.Session.Username, Action: action, Target: id})
	if err != nil {
		return PeerResult{}, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	var out PeerResult
	requestErr := s.agent(ctx, "/peers/"+id, map[string]string{"action": action}, &out)
	status := "completed"
	if requestErr != nil {
		status = "failed"
	}
	// Persist the result even if the browser disconnects during a key change.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.db.FinishAudit(finishCtx, db.FinishAuditParams{ID: auditID, Status: status}); err != nil {
		slog.ErrorContext(ctx, "audit completion failed", "id", auditID)
		return PeerResult{}, huma.Error503ServiceUnavailable("audit.unknown_outcome")
	}
	if requestErr != nil {
		return PeerResult{}, huma.Error502BadGateway("peer.change_failed_check_state")
	}
	return out, nil
}
