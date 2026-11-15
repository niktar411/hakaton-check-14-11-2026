package worker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	worker, err := NewWorker(testConfig(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewAgent(AgentConfig{
		Token: strings.Repeat("t", 32), CallerIP: "10.77.0.1", BindIP: "10.77.0.10",
	}, worker)
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

func call(t *testing.T, agent *Agent, method, path, remote, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.RemoteAddr = remote + ":50000"
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	agent.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestAgentBoundaryRejectsForeignCallers(t *testing.T) {
	agent := newTestAgent(t)
	if recorder := call(t, agent, http.MethodGet, "/metrics", "10.77.0.99", strings.Repeat("t", 32), ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("foreign caller = %d", recorder.Code)
	}
	if recorder := call(t, agent, http.MethodGet, "/metrics", "10.77.0.1", "wrong", ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("bad token = %d", recorder.Code)
	}
	if recorder := call(t, agent, http.MethodGet, "/health", "127.0.0.1", "", ""); recorder.Code != http.StatusOK {
		t.Fatalf("local health = %d", recorder.Code)
	}
}

func TestAgentToolAndDeploymentValidation(t *testing.T) {
	agent := newTestAgent(t)
	token := strings.Repeat("t", 32)
	if recorder := call(t, agent, http.MethodPost, "/tools/unknown", "10.77.0.1", token, `{}`); !strings.Contains(recorder.Body.String(), "invalid_operation") {
		t.Fatalf("unknown tool = %s", recorder.Body.String())
	}
	if recorder := call(t, agent, http.MethodPost, "/deployments/deploy", "10.77.0.1", token, `{"target":"production"}`); !strings.Contains(recorder.Body.String(), "invalid_operation") {
		t.Fatalf("worker deploy = %s", recorder.Body.String())
	}
	if recorder := call(t, agent, http.MethodGet, "/missing", "10.77.0.1", token, ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("missing route = %d", recorder.Code)
	}
	if recorder := call(t, agent, http.MethodPost, "/tools/list_files", "10.77.0.1", token, "not-json"); !strings.Contains(recorder.Body.String(), "invalid_request") {
		t.Fatalf("invalid body = %s", recorder.Body.String())
	}
}
