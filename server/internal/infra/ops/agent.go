package ops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hackathon/internal/infra/metrics"
)

// AgentConfig is the restricted SERVER 1 control agent configuration.
type AgentConfig struct {
	Token       string
	Callers     map[string]bool
	WorkerURL   string
	WorkerToken string
	VPN         *VPN
	Production  func() (*Production, error)
}

// Agent serves the fixed control API on 10.77.0.1:9101.
type Agent struct {
	config AgentConfig
	client *http.Client
}

// NewAgent validates the control boundary.
func NewAgent(config AgentConfig) (*Agent, error) {
	if len(config.Token) < 32 {
		return nil, errors.New("AGENT_TOKEN must be configured")
	}
	if len(config.Callers) == 0 || config.VPN == nil || config.Production == nil {
		return nil, errors.New("agent callers, VPN and production are required")
	}
	return &Agent{
		config: config,
		client: &http.Client{Timeout: 90 * time.Second, CheckRedirect: noRedirect},
	}, nil
}

// Handler returns the control agent HTTP handler.
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.serve)
	return mux
}

func (a *Agent) serve(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		writeAgentJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.get(w, r)
	case http.MethodPost:
		a.post(w, r)
	default:
		writeAgentJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method_not_allowed"})
	}
}

func (a *Agent) authorized(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !a.config.Callers[host] {
		return false
	}
	return strings.EqualFold(r.Header.Get("Authorization"), "Bearer "+a.config.Token)
}

func (a *Agent) get(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/metrics":
		containers := a.containers(r.Context())
		host, err := metrics.HostMetrics("Сервер 1 · GitLab", containers)
		if err != nil {
			writeAgentJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "agent_unavailable"})
			return
		}
		writeAgentJSON(w, http.StatusOK, host)
	case "/peers":
		peers, err := a.config.VPN.Inventory()
		if err != nil {
			writeAgentJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "agent_unavailable"})
			return
		}
		writeAgentJSON(w, http.StatusOK, peers)
	case "/containers", "/metrics/containers":
		writeAgentJSON(w, http.StatusOK, map[string]any{"containers": a.containers(r.Context())})
	default:
		writeAgentJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
	}
}

func (a *Agent) containers(ctx context.Context) []metrics.Container {
	runner := func(args []string) (string, error) {
		result, err := runAgentCommand(ctx, args)
		if err != nil {
			return "", err
		}
		return result, nil
	}
	containers, err := metrics.Collect("server1", runner)
	if err != nil {
		return []metrics.Container{}
	}
	return containers
}

func (a *Agent) post(w http.ResponseWriter, r *http.Request) {
	body, err := readAgentBody(r)
	if err != nil {
		writeAgentJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/peers/"):
		action, _ := body["action"].(string)
		result, err := a.config.VPN.Change(strings.TrimPrefix(r.URL.Path, "/peers/"), action)
		if err != nil {
			writeAgentJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_operation"})
			return
		}
		writeAgentJSON(w, http.StatusOK, result)
	case strings.HasPrefix(r.URL.Path, "/deployments/"):
		a.deployment(w, r, body)
	default:
		writeAgentJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
	}
}

func (a *Agent) deployment(w http.ResponseWriter, r *http.Request, body map[string]any) {
	action := strings.TrimPrefix(r.URL.Path, "/deployments/")
	switch action {
	case "deploy", "rollback", "restart", "start", "stop", "create_test_database", "drop_test_database":
	default:
		writeAgentJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_operation"})
		return
	}
	target, _ := body["target"].(string)
	release, _ := body["release"].(string)
	if target != "production" && target != "coding" && target != "review" && target != "test" {
		writeAgentJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_operation"})
		return
	}
	if target != "production" {
		if release != "" {
			writeAgentJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_operation"})
			return
		}
		result, status, err := a.workerDeployment(action, target)
		if err != nil {
			writeAgentJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "worker_unavailable"})
			return
		}
		writeAgentJSON(w, status, result)
		return
	}
	production, err := a.config.Production()
	if err != nil {
		writeAgentJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "production_unavailable"})
		return
	}
	result, err := production.Run(r.Context(), action, release)
	if err != nil {
		var productionErr *ProductionError
		if errors.As(err, &productionErr) {
			if productionErr.Result != nil {
				writeAgentJSON(w, http.StatusServiceUnavailable, productionErr.Result)
				return
			}
		}
		writeAgentJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "operation_failed"})
		return
	}
	writeAgentJSON(w, http.StatusOK, result)
}

func (a *Agent) workerDeployment(action, target string) (map[string]any, int, error) {
	base, err := url.Parse(strings.TrimRight(a.config.WorkerURL, "/"))
	if err != nil || base.Scheme != "http" || base.Hostname() != "10.77.0.10" ||
		base.Path != "" || a.config.WorkerToken == "" {
		return nil, http.StatusServiceUnavailable, errors.New("invalid worker deployment configuration")
	}
	payload, _ := json.Marshal(map[string]string{"target": target, "release": ""})
	request, err := http.NewRequest(http.MethodPost, base.String()+"/deployments/"+action, strings.NewReader(string(payload)))
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	request.Header.Set("Authorization", "Bearer "+a.config.WorkerToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, 131073))
	if err != nil || len(content) > 131072 {
		return nil, http.StatusServiceUnavailable, errors.New("worker response too large")
	}
	var decoded map[string]any
	if err := json.Unmarshal(content, &decoded); err != nil {
		return nil, http.StatusServiceUnavailable, errors.New("worker operation failed")
	}
	if response.StatusCode >= 400 {
		return decoded, http.StatusServiceUnavailable, nil
	}
	status, _ := decoded["status"].(string)
	if status == "" {
		status = "failed"
	}
	return map[string]any{
		"status": status, "backup_name": "",
		"details": map[string]any{"target": target},
	}, http.StatusOK, nil
}

func readAgentBody(r *http.Request) (map[string]any, error) {
	if len(r.TransferEncoding) > 0 || r.ContentLength <= 0 || r.ContentLength > 4096 {
		return nil, errors.New("invalid request")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.UseNumber()
	var body map[string]any
	if err := decoder.Decode(&body); err != nil || body == nil {
		return nil, errors.New("invalid request")
	}
	return body, nil
}

func writeAgentJSON(w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) > 128*1024 {
		status = http.StatusRequestEntityTooLarge
		payload = []byte(`{"error":"response_too_large"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
