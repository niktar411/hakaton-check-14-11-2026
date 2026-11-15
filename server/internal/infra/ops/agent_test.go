package ops

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	directory := t.TempDir()
	agent, err := NewAgent(AgentConfig{
		Token:   strings.Repeat("t", 32),
		Callers: map[string]bool{"10.77.0.1": true, "10.77.0.10": true, "172.30.0.5": true},
		VPN:     NewVPN(VPNConfig{ConfigPath: directory + "/awg0.conf", Profiles: directory, StatePath: directory + "/state.json"}),
		Production: func() (*Production, error) {
			return nil, errors.New("not configured")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

func callAgent(t *testing.T, agent *Agent, method, path, remote, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.RemoteAddr = remote + ":50000"
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	agent.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestControlAgentBoundary(t *testing.T) {
	agent := newTestAgent(t)
	if recorder := callAgent(t, agent, http.MethodGet, "/peers", "10.77.0.99", strings.Repeat("t", 32), ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("foreign caller = %d", recorder.Code)
	}
	if recorder := callAgent(t, agent, http.MethodGet, "/peers", "10.77.0.1", "wrong", ""); recorder.Code != http.StatusForbidden {
		t.Fatalf("bad token = %d", recorder.Code)
	}
	if recorder := callAgent(t, agent, http.MethodGet, "/missing", "10.77.0.1", strings.Repeat("t", 32), ""); recorder.Code != http.StatusNotFound {
		t.Fatalf("missing route = %d", recorder.Code)
	}
}

func TestControlAgentValidatesDeployments(t *testing.T) {
	agent := newTestAgent(t)
	token := strings.Repeat("t", 32)
	for _, body := range []string{`{"target":"unknown"}`, `{"target":"coding","release":"image"}`, `{}`} {
		recorder := callAgent(t, agent, http.MethodPost, "/deployments/restart", "10.77.0.1", token, body)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %s = %d", body, recorder.Code)
		}
	}
	if recorder := callAgent(t, agent, http.MethodPost, "/deployments/deploy", "10.77.0.1", token, `{"target":"coding"}`); recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured worker = %d", recorder.Code)
	}
}

func TestControlAgentRequiresConfiguration(t *testing.T) {
	_, err := NewAgent(AgentConfig{Token: "short"})
	if err == nil {
		t.Fatal("short token accepted")
	}
}
