package deployments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

var agentPaths = map[string]string{
	ActionDeploy:             "/deployments/deploy",
	ActionRollback:           "/deployments/rollback",
	ActionRestart:            "/deployments/restart",
	ActionStart:              "/deployments/start",
	ActionStop:               "/deployments/stop",
	ActionCreateTestDatabase: "/deployments/create_test_database",
	ActionDropTestDatabase:   "/deployments/drop_test_database",
}

// Orchestrator executes only the fixed operations understood by the agent.
// Implementations must honor ctx. For production deploy, restart and rollback,
// a completed result certifies that the agent created a backup before changing
// the service and completed its health-check/rollback transaction.
type Orchestrator interface {
	Run(ctx context.Context, action, target, release string) (AgentResult, error)
}

// AgentOrchestrator is an authenticated HTTP client for the restricted agent.
// It has no command or endpoint parameter supplied by callers.
type AgentOrchestrator struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewOrchestrator(config Config, client *http.Client) *AgentOrchestrator {
	if client == nil {
		client = &http.Client{}
	}
	// Clone the caller's client and always disable redirects so a redirected
	// request cannot forward the agent credential to another endpoint.
	clone := *client
	clone.Timeout = operationTimeout
	clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &AgentOrchestrator{
		baseURL: strings.TrimRight(config.AgentURL, "/"),
		token:   config.AgentToken,
		http:    &clone,
	}
}

// AgentError reports only an HTTP status, never an untrusted agent response
// body which could contain credentials or command output.
type AgentError struct {
	StatusCode int
}

func (e *AgentError) Error() string {
	return fmt.Sprintf("deployment agent returned status %d", e.StatusCode)
}

func (c *AgentOrchestrator) Run(ctx context.Context, action, target, release string) (AgentResult, error) {
	path, ok := agentPaths[action]
	if !ok || !validTarget(target) || !validRelease(release) || c.baseURL == "" || c.token == "" {
		return AgentResult{}, fmt.Errorf("invalid deployment agent configuration or operation")
	}
	payload, err := json.Marshal(struct {
		Target  string `json:"target"`
		Release string `json:"release,omitempty"`
	}{Target: target, Release: release})
	if err != nil {
		return AgentResult{}, fmt.Errorf("encode deployment agent request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return AgentResult{}, fmt.Errorf("create deployment agent request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return AgentResult{}, fmt.Errorf("call deployment agent: %w", err)
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			slog.WarnContext(ctx, "close deployment agent response failed", "error", closeErr)
		}
	}()

	var wire struct {
		Status          string          `json:"status"`
		BackupName      string          `json:"backup_name"`
		BackupNameCamel string          `json:"backupName"`
		Backup          string          `json:"backup"`
		Details         json.RawMessage `json:"details"`
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 128*1024))
	decodeErr := decoder.Decode(&wire)
	if decodeErr == nil {
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			decodeErr = fmt.Errorf("multiple JSON values in deployment agent response")
		}
	}
	result := AgentResult{Status: wire.Status, BackupName: wire.BackupName, Details: wire.Details}
	if result.BackupName == "" {
		result.BackupName = wire.BackupNameCamel
	}
	if res.StatusCode != http.StatusOK {
		// A non-200 response can still contain a terminal rolled_back result and
		// backup name. Preserve those known fields for the audit trail.
		return result, &AgentError{StatusCode: res.StatusCode}
	}
	if decodeErr != nil {
		return AgentResult{}, fmt.Errorf("decode deployment agent response: %w", decodeErr)
	}
	return result, nil
}
