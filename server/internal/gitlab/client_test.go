package gitlab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMergeRequestUsesProjectAPIAndRejectsOutsideURL(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.EscapedPath(), "hackathon%2Fhackathon") || r.Header.Get("PRIVATE-TOKEN") != "token" {
			t.Errorf("unexpected GitLab request: %s", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(MergeRequest{IID: 7, State: "opened", WebURL: server.URL + "/hackathon/hackathon/-/merge_requests/7"}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewClient(Config{URL: server.URL, Token: "token", Project: "hackathon/hackathon"}, server.Client())
	result, err := client.MergeRequestByIID(t.Context(), 7)
	if err != nil || result.IID != 7 {
		t.Fatalf("valid merge request rejected: %+v, %v", result, err)
	}
}

func TestMergeRequestRejectsPhishingURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(MergeRequest{IID: 7, WebURL: "https://attacker.invalid/merge"}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewClient(Config{URL: server.URL, Token: "token", Project: "group/project"}, server.Client())
	if _, err := client.MergeRequestByIID(t.Context(), 7); err == nil {
		t.Fatal("outside merge request URL accepted")
	}
}

func TestRedirectsNeverReceiveGitLabToken(t *testing.T) {
	redirectCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectCalls++
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()
	client := NewClient(Config{URL: server.URL, Token: "token", Project: "group/project"}, server.Client())
	if _, err := client.MergeRequestByIID(t.Context(), 7); err == nil {
		t.Fatal("redirect response accepted")
	}
	if redirectCalls != 0 {
		t.Fatal("GitLab token forwarded through redirect")
	}
}
