package orchestrator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigurationRequiresExactWorkerAddress(t *testing.T) {
	if _, err := NewServer(Config{Token: strings.Repeat("t", 32), WorkerToken: strings.Repeat("w", 32), Upstream: "http://example.com:9102"}); err == nil {
		t.Fatal("external upstream accepted")
	}
	if _, err := NewServer(Config{Token: "short", WorkerToken: strings.Repeat("w", 32), Upstream: "http://10.77.0.10:9102"}); err == nil {
		t.Fatal("short token accepted")
	}
	if _, err := NewServer(Config{Token: strings.Repeat("t", 32), WorkerToken: strings.Repeat("w", 32), Upstream: "http://10.77.0.10:9102"}); err != nil {
		t.Fatalf("valid configuration rejected: %v", err)
	}
}

func TestHealthAndAuthorization(t *testing.T) {
	server := newServer(Config{Token: strings.Repeat("t", 32), WorkerToken: strings.Repeat("w", 32), Upstream: "http://127.0.0.1:1"})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("health = %d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unauthorized = %d", recorder.Code)
	}
}

func TestForwardFiltersMetricsAndTools(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("w", 32) {
			t.Error("worker token missing")
		}
		switch r.URL.Path {
		case "/metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "Сервер 2", "available": true, "secret": "must-not-pass",
				"containers": []any{map[string]any{"name": "coding", "server": "server2", "secret": "x"}},
			})
		case "/tools/search_code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true, "secret": "must-not-pass",
				"matches": []any{map[string]any{"path": "a.go", "line": 3, "text": "x", "secret": "y"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	server := newServer(Config{Token: strings.Repeat("t", 32), WorkerToken: strings.Repeat("w", 32), Upstream: upstream.URL})

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 32))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics = %d", recorder.Code)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "secret") || !strings.Contains(body, "coding") {
		t.Fatalf("metrics body = %s", body)
	}

	request = httptest.NewRequest(http.MethodPost, "/tools/search_code", strings.NewReader(`{"taskId":1,"query":"x"}`))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 32))
	request.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	body = recorder.Body.String()
	if recorder.Code != http.StatusOK || strings.Contains(body, "secret") || !strings.Contains(body, "a.go") {
		t.Fatalf("tool response = %d %s", recorder.Code, body)
	}
}

func TestUpstreamFailureIsUnavailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed", http.StatusInternalServerError)
	}))
	defer upstream.Close()
	server := newServer(Config{Token: strings.Repeat("t", 32), WorkerToken: strings.Repeat("w", 32), Upstream: upstream.URL})
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("t", 32))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("upstream failure = %d", recorder.Code)
	}
}
