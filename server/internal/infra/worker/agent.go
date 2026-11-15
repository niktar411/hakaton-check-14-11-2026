package worker

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hackathon/internal/infra/metrics"
)

// AgentConfig is the VPN-only HTTP boundary of the worker.
type AgentConfig struct {
	Token    string
	CallerIP string
	BindIP   string
}

// Agent serves the worker JSON API.
type Agent struct {
	config AgentConfig
	worker *Worker
}

// NewAgent validates the security boundary and builds the HTTP agent.
func NewAgent(config AgentConfig, worker *Worker) (*Agent, error) {
	if len(config.Token) < 32 || !strings.HasPrefix(config.BindIP, "10.77.") || config.CallerIP != "10.77.0.1" {
		return nil, errors.New("WORKER_TOKEN and exact VPN bind/caller addresses must be configured")
	}
	return &Agent{config: config, worker: worker}, nil
}

// Handler returns the worker HTTP handler.
func (a *Agent) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.serve)
	return mux
}

func (a *Agent) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host == a.config.CallerIP || host == "127.0.0.1" || host == a.config.BindIP {
			writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
			return
		}
	}
	if !a.authorized(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.get(w, r)
	case http.MethodPost:
		a.post(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
	}
}

func (a *Agent) authorized(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host != a.config.CallerIP {
		return false
	}
	return strings.EqualFold(r.Header.Get("Authorization"), "Bearer "+a.config.Token)
}

func (a *Agent) get(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/metrics":
		containers := a.containers()
		host, err := metrics.HostMetrics("Сервер 2 · coding, review и test", containers)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "metrics_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, host)
	case "/containers", "/metrics/containers":
		writeJSON(w, http.StatusOK, map[string]any{"containers": a.containers()})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (a *Agent) containers() []metrics.Container {
	runner := func(args []string) (string, error) {
		result, err := RunBounded(args, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 256 * 1024})
		if err != nil {
			return "", err
		}
		if result.ReturnCode != 0 {
			return "", workerError("docker_unavailable", 503)
		}
		return result.Output, nil
	}
	containers, err := metrics.Collect("server2", runner)
	if err != nil {
		return []metrics.Container{}
	}
	return containers
}

func (a *Agent) post(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "invalid_request"})
		return
	}
	var result map[string]any
	switch {
	case strings.HasPrefix(r.URL.Path, "/tools/"):
		operation := strings.TrimPrefix(r.URL.Path, "/tools/")
		if !Tools[operation] {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "invalid_operation"})
			return
		}
		result, err = a.worker.Execute(operation, body)
	case strings.HasPrefix(r.URL.Path, "/deployments/"):
		action := strings.TrimPrefix(r.URL.Path, "/deployments/")
		if !DeploymentActions[action] {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "invalid_operation"})
			return
		}
		target, _ := body["target"].(string)
		result, err = a.worker.Deployment(action, target)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		var workerErr Error
		if errors.As(err, &workerErr) {
			status := http.StatusOK
			if workerErr.Status >= 500 {
				status = workerErr.Status
			}
			writeJSON(w, status, map[string]any{"ok": false, "status": "failed", "error": workerErr.Code})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "worker_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func readBody(r *http.Request) (map[string]any, error) {
	if len(r.TransferEncoding) > 0 {
		return nil, errors.New("chunked request")
	}
	if r.ContentLength <= 0 || r.ContentLength > 300000 {
		return nil, errors.New("invalid request size")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 300001))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("invalid request")
	}
	return value, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) > ResponseLimit {
		status = http.StatusRequestEntityTooLarge
		payload = []byte(`{"ok":false,"error":"response_too_large"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
