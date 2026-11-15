package codingtasks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPWorkerAuthenticatedFixedOperationContract(t *testing.T) {
	const token = "worker-secret-for-test"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/tools/run_check" || r.Method != http.MethodPost {
			t.Errorf("unexpected worker request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("worker bearer token missing")
		}
		var input ToolInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input.TaskID != 9 || input.Check != CheckServerTest {
			t.Errorf("unexpected input: %+v", input)
		}
		body, _ := json.Marshal(input)
		if strings.Contains(string(body), token) {
			t.Error("worker token duplicated into operation input")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ToolOutput{OK: true, Passed: false, Output: "test failed"}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	worker, err := NewHTTPWorker(server.URL, token, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	output, err := worker.Execute(t.Context(), ToolRunCheck, ToolInput{TaskID: 9, Check: CheckServerTest})
	if err != nil {
		t.Fatal(err)
	}
	if !output.OK || output.Passed || output.Output != "test failed" {
		t.Fatalf("check result changed: %+v", output)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestHTTPWorkerRejectsSecretsAndArbitraryOperationsLocally(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if err := json.NewEncoder(w).Encode(ToolOutput{OK: true}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	worker, err := NewHTTPWorker(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		operation string
		input     ToolInput
	}{
		{ToolReadFile, ToolInput{TaskID: 1, Path: ".env"}},
		{ToolWriteFile, ToolInput{TaskID: 1, Path: "..\\outside", Content: "x"}},
		{ToolReadFile, ToolInput{TaskID: 1, Path: "/absolute"}},
		{ToolSearchCode, ToolInput{TaskID: 1, Query: "PRIVATE KEY", Path: ""}},
		{ToolRunCheck, ToolInput{TaskID: 1, Check: "arbitrary-shell"}},
		{"execute_command", ToolInput{TaskID: 1, Content: "id"}},
	} {
		if _, err := worker.Execute(t.Context(), test.operation, test.input); err == nil {
			t.Fatalf("%s with %+v accepted", test.operation, test.input)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("rejected operation reached HTTP worker")
	}
}

func TestHTTPWorkerDoesNotForwardTokenAcrossRedirect(t *testing.T) {
	var redirectedRequests atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		redirectedRequests.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("bearer token forwarded to redirect target")
		}
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", destination.URL+r.URL.Path)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	worker, err := NewHTTPWorker(source.URL, "token", source.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Execute(t.Context(), ToolListFiles, ToolInput{TaskID: 1, Path: ""}); err == nil {
		t.Fatal("redirect response accepted")
	}
	if redirectedRequests.Load() != 0 {
		t.Fatal("redirect target was called")
	}
}

func TestHTTPWorkerRejectsMalformedOrUnsafeOutput(t *testing.T) {
	for _, body := range []string{
		`{"ok":true,"unexpected":"value"}`,
		`{"ok":true} {}`,
		`{"ok":true,"files":[".env"]}`,
		`{"ok":true,"mergeRequestIid":0,"mergeRequestUrl":""}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		worker, err := NewHTTPWorker(server.URL, "token", server.Client())
		if err != nil {
			t.Fatal(err)
		}
		operation := ToolListFiles
		if strings.Contains(body, "mergeRequest") {
			operation = ToolCreateMergeRequest
		}
		input := ToolInput{TaskID: 1, Path: ""}
		if operation == ToolCreateMergeRequest {
			input = ToolInput{TaskID: 1, Title: "Title", Description: "Description"}
		}
		if _, err := worker.Execute(t.Context(), operation, input); err == nil {
			t.Fatalf("unsafe worker output accepted: %s", body)
		}
		server.Close()
	}
}

func TestWorkerURLAndTokenValidation(t *testing.T) {
	for _, baseURL := range []string{"", "file:///tmp/worker", "https://user:password@example.test", "https://example.test/path?redirect=other"} {
		if _, err := NewHTTPWorker(baseURL, "token", nil); err == nil {
			t.Fatalf("unsafe worker URL accepted: %q", baseURL)
		}
	}
	if _, err := NewHTTPWorker("https://example.test", "bad\r\nAuthorization: none", nil); err == nil {
		t.Fatal("header injection token accepted")
	}
}
