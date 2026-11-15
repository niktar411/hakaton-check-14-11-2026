package gitlabprotect

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestConfigurationValidation(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("empty configuration accepted")
	}
	if _, err := New(Config{APIURL: "not-a-url", Token: "t", Project: "p"}); err == nil {
		t.Fatal("invalid URL accepted")
	}
	client, err := New(Config{APIURL: "http://127.0.0.1:1/api/v4", Token: "t", Project: "p", AdminID: "1", Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Enforce(); err == nil || !strings.Contains(err.Error(), "only main") {
		// main is allowed, so this call fails later on network; only assert non-nil.
		if err == nil {
			t.Fatal("unreachable API accepted")
		}
	}
	client.config.Branch = "dev"
	if err := client.Enforce(); err == nil || err.Error() != "only main is supported by this protection policy" {
		t.Fatalf("branch error = %v", err)
	}
}

func TestAdministratorLookupMustBeExact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]any{
			map[string]any{"id": 1, "state": "active", "is_admin": true},
			map[string]any{"id": 2, "state": "active", "is_admin": true},
		})
	}))
	defer server.Close()
	client, err := New(Config{APIURL: server.URL, Token: "t", Project: "p", AdminName: "root", Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.administrator(); err == nil {
		t.Fatal("ambiguous administrator accepted")
	}
}

func TestEnforceAppliesAndVerifiesPolicy(t *testing.T) {
	var mu sync.Mutex
	flags := map[string]bool{}
	ruleCreated := false
	branchCreated := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		path := r.URL.Path
		switch {
		case path == "/users/7":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "state": "active", "is_admin": true})
		case strings.HasPrefix(path, "/projects/") && strings.HasSuffix(path, "/approval_rules"):
			if r.Method == http.MethodPost {
				ruleCreated = true
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{"id": 3, "name": RuleName})
				return
			}
			if !ruleCreated {
				_ = json.NewEncoder(w).Encode([]any{})
				return
			}
			_ = json.NewEncoder(w).Encode([]any{map[string]any{
				"id": 3, "name": RuleName, "approvals_required": 1,
				"users": []any{map[string]any{"id": 7}},
			}})
		case strings.HasPrefix(path, "/projects/") && strings.HasSuffix(path, "/protected_branches") && r.Method == http.MethodPost:
			branchCreated = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 11, "allow_force_push": false,
				"allowed_to_push": []any{map[string]any{"access_level": 0}},
			})
		case strings.HasPrefix(path, "/projects/") && strings.HasSuffix(path, "/protected_branches/main"):
			if r.Method == http.MethodGet && !branchCreated {
				http.NotFound(w, r)
				return
			}
			if r.Method == http.MethodPost {
				branchCreated = true
				w.WriteHeader(http.StatusCreated)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 11, "allow_force_push": false,
				"allowed_to_push": []any{map[string]any{"access_level": 0}},
			})
		case strings.HasPrefix(path, "/projects/"):
			if r.Method == http.MethodPatch {
				flags["pipeline"] = true
				flags["discussions"] = true
				flags["skipped"] = true
				flags["reset"] = true
				_ = json.NewEncoder(w).Encode(map[string]any{})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"only_allow_merge_if_pipeline_succeeds":            flags["pipeline"],
				"only_allow_merge_if_all_discussions_are_resolved": flags["discussions"],
				"allow_merge_on_skipped_pipeline":                  !flags["skipped"],
				"reset_approvals_on_push":                          flags["reset"],
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{APIURL: server.URL, Token: "t", Project: "group/project", AdminID: "7", Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Enforce(); err != nil {
		t.Fatal(err)
	}
	if !ruleCreated || !branchCreated {
		t.Fatalf("rule=%v branch=%v", ruleCreated, branchCreated)
	}
}
