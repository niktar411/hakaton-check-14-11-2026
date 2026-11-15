// Package orchestrator relays fixed tool calls from SERVER 1 to the worker.
package orchestrator

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxRequest  = 300000
	maxResponse = 128 * 1024
)

// Config is the internal relay configuration.
type Config struct {
	Token       string
	WorkerToken string
	Upstream    string
}

// Tools are the only relayed worker operations.
var Tools = map[string]bool{
	"prepare_workspace": true, "list_files": true, "read_file": true,
	"search_code": true, "write_file": true, "run_check": true,
	"create_merge_request": true,
}

var metricKeys = map[string]bool{
	"name": true, "available": true, "error": true, "cpuPercent": true,
	"memoryUsed": true, "memoryTotal": true, "diskUsed": true, "diskTotal": true,
	"uptimeSeconds": true, "sampledAt": true, "containers": true,
}

var containerKeys = map[string]bool{
	"server": true, "name": true, "image": true, "state": true, "health": true,
	"cpuPercent": true, "memoryUsed": true, "memoryLimit": true, "restarts": true,
}

var matchKeys = map[string]bool{"path": true, "line": true, "text": true}

var toolKeys = map[string]bool{
	"ok": true, "files": true, "content": true, "matches": true, "output": true,
	"passed": true, "mergeRequestIid": true, "mergeRequestUrl": true, "error": true,
}

// Server is the relay HTTP server.
type Server struct {
	config Config
	client *http.Client
}

// NewServer validates the internal relay boundary.
func NewServer(config Config) (*Server, error) {
	if len(config.Token) < 32 || len(config.WorkerToken) < 32 {
		return nil, errors.New("orchestrator tokens must be configured")
	}
	parsed, err := url.Parse(config.Upstream)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "10.77.0.10" || parsed.Path != "" {
		return nil, errors.New("exact SERVER 2 VPN URL must be configured")
	}
	return newServer(config), nil
}

func newServer(config Config) *Server {
	return &Server{
		config: config,
		client: &http.Client{
			Timeout: 115 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Handler returns the relay HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serve)
	return mux
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		return
	}
	if !s.authorized(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/metrics" {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		s.forward(w, r, nil)
	case http.MethodPost:
		operation := strings.TrimPrefix(r.URL.Path, "/tools/")
		if !strings.HasPrefix(r.URL.Path, "/tools/") || !Tools[operation] {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		if len(r.TransferEncoding) > 0 || r.ContentLength <= 0 || r.ContentLength > maxRequest {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_request"})
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequest+1))
		if err != nil || len(body) > maxRequest {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_request"})
			return
		}
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil || value == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_request"})
			return
		}
		s.forward(w, r, body)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method_not_allowed"})
	}
}

func (s *Server) authorized(r *http.Request) bool {
	return len(s.config.Token) > 0 &&
		strings.EqualFold(r.Header.Get("Authorization"), "Bearer "+s.config.Token)
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, body []byte) {
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}
	request, err := http.NewRequest(method, s.config.Upstream+r.URL.Path, strings.NewReader(string(body)))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "worker_unavailable"})
		return
	}
	request.Header.Set("Authorization", "Bearer "+s.config.WorkerToken)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "worker_unavailable"})
		return
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "worker_unavailable"})
		return
	}
	if len(content) > maxResponse {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"ok": false, "error": "response_too_large"})
		return
	}
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil || value == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "worker_response_invalid"})
		return
	}
	status := response.StatusCode
	if status >= 400 {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, s.filter(r.URL.Path, value))
}

func (s *Server) filter(path string, value map[string]any) map[string]any {
	if path == "/metrics" {
		filtered := map[string]any{}
		for key, item := range value {
			if metricKeys[key] {
				filtered[key] = item
			}
		}
		containers := []any{}
		if items, ok := filtered["containers"].([]any); ok {
			for _, item := range items {
				container, ok := item.(map[string]any)
				if !ok {
					continue
				}
				clean := map[string]any{}
				for key, field := range container {
					if containerKeys[key] {
						clean[key] = field
					}
				}
				containers = append(containers, clean)
				if len(containers) == 100 {
					break
				}
			}
		}
		filtered["containers"] = containers
		return filtered
	}
	filtered := map[string]any{}
	for key, item := range value {
		if toolKeys[key] {
			filtered[key] = item
		}
	}
	if items, ok := filtered["matches"].([]any); ok {
		matches := []any{}
		for _, item := range items {
			match, ok := item.(map[string]any)
			if !ok {
				continue
			}
			clean := map[string]any{}
			for key, field := range match {
				if matchKeys[key] {
					clean[key] = field
				}
			}
			matches = append(matches, clean)
			if len(matches) == 100 {
				break
			}
		}
		filtered["matches"] = matches
	}
	return filtered
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) > maxResponse {
		status = http.StatusRequestEntityTooLarge
		payload = []byte(`{"ok":false,"error":"response_too_large"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}
