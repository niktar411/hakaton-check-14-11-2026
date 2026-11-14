package codingtasks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ToolPrepareWorkspace   = "prepare_workspace"
	ToolListFiles          = "list_files"
	ToolReadFile           = "read_file"
	ToolSearchCode         = "search_code"
	ToolWriteFile          = "write_file"
	ToolRunCheck           = "run_check"
	ToolCreateMergeRequest = "create_merge_request"
)

const (
	CheckBunCheck    = "bun-check"
	CheckClientBuild = "client-build"
	CheckServerTest  = "server-test"
	CheckServerVet   = "server-vet"
)

// ToolInput is the complete worker wire contract. TaskID is injected by the
// server for repository operations; a model cannot select another workspace.
type ToolInput struct {
	TaskID         int64  `json:"taskId"`
	ParentTaskID   int64  `json:"parentTaskId,omitempty"`
	Path           string `json:"path,omitempty"`
	Query          string `json:"query,omitempty"`
	Content        string `json:"content,omitempty"`
	Check          string `json:"check,omitempty"`
	Title          string `json:"title,omitempty"`
	Description    string `json:"description,omitempty"`
	OwnerID        int64  `json:"ownerId,omitempty"`
	PlannerBlockID int64  `json:"plannerBlockId,omitempty"`
	Position       *int32 `json:"position,omitempty"`
	Branch         string `json:"branch,omitempty"`
	MRIID          *int64 `json:"mrIid,omitempty"`
	SHA            string `json:"sha,omitempty"`
	Action         string `json:"action,omitempty"`
	Target         string `json:"target,omitempty"`
}

// SearchMatch is one bounded code-search result.
type SearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// ToolOutput is intentionally fixed: workers cannot return arbitrary commands,
// credentials, redirects, or a second operation for the application to run.
type ToolOutput struct {
	OK              bool          `json:"ok"`
	Files           []string      `json:"files,omitempty"`
	Content         string        `json:"content,omitempty"`
	Matches         []SearchMatch `json:"matches,omitempty"`
	Output          string        `json:"output,omitempty"`
	Passed          bool          `json:"passed,omitempty"`
	MergeRequestIID int64         `json:"mergeRequestIid,omitempty"`
	MergeRequestURL string        `json:"mergeRequestUrl,omitempty"`
	Error           string        `json:"error,omitempty"`
}

// Worker is implemented by HTTPWorker and by deterministic fakes in tests.
type Worker interface {
	Execute(ctx context.Context, operation string, input ToolInput) (ToolOutput, error)
}

// HTTPWorker calls the restricted worker on server 2. Redirects are disabled
// so its bearer token can never be forwarded to another origin.
type HTTPWorker struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewHTTPWorker(baseURL, token string, client *http.Client) (*HTTPWorker, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid worker URL")
	}
	if token == "" || len(token) > 4096 || strings.TrimSpace(token) != token || strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("invalid worker token")
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	copyClient := *client
	if copyClient.Timeout == 0 {
		copyClient.Timeout = 2 * time.Minute
	}
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &HTTPWorker{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: &copyClient}, nil
}

func (w *HTTPWorker) Execute(ctx context.Context, operation string, input ToolInput) (ToolOutput, error) {
	if err := validateToolInput(operation, input); err != nil {
		return ToolOutput{}, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("encode worker input: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL+"/tools/"+operation, bytes.NewReader(data))
	if err != nil {
		return ToolOutput{}, fmt.Errorf("create worker request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := w.http.Do(req)
	if err != nil {
		return ToolOutput{}, fmt.Errorf("call worker: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ToolOutput{}, fmt.Errorf("worker rejected operation with status %d", res.StatusCode)
	}
	var output ToolOutput
	decoder := json.NewDecoder(io.LimitReader(res.Body, (2<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return ToolOutput{}, fmt.Errorf("decode worker response: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return ToolOutput{}, fmt.Errorf("decode worker response: %w", err)
	}
	if len(output.Content) > 1<<20 || len(output.Output) > 1<<20 || len(output.Files) > 10000 || len(output.Matches) > 10000 {
		return ToolOutput{}, fmt.Errorf("worker response exceeds limits")
	}
	if err := validateToolOutput(operation, output); err != nil {
		return ToolOutput{}, err
	}
	return output, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateToolInput(operation string, input ToolInput) error {
	if input.TaskID < 1 || input.ParentTaskID < 0 {
		return ErrInvalid
	}
	switch operation {
	case ToolPrepareWorkspace:
		return nil
	case ToolListFiles:
		return validateRelativePath(input.Path, true)
	case ToolReadFile:
		return validateRelativePath(input.Path, false)
	case ToolSearchCode:
		query, err := validText(input.Query, 1000)
		if err != nil || query != strings.TrimSpace(input.Query) || secretQuery(input.Query) {
			return ErrInvalid
		}
		return validateRelativePath(input.Path, true)
	case ToolWriteFile:
		if err := validateRelativePath(input.Path, false); err != nil {
			return err
		}
		if len(input.Content) > 256<<10 || !utf8.ValidString(input.Content) || strings.ContainsRune(input.Content, '\x00') {
			return ErrInvalid
		}
		return nil
	case ToolRunCheck:
		if !validCheck(input.Check) {
			return ErrInvalid
		}
		return nil
	case ToolCreateMergeRequest:
		if _, err := validText(input.Title, 200); err != nil {
			return err
		}
		if len(input.Description) > 16000 || !utf8.ValidString(input.Description) || strings.ContainsRune(input.Description, '\x00') {
			return ErrInvalid
		}
		return nil
	default:
		return ErrInvalid
	}
}

func validCheck(check string) bool {
	switch check {
	case CheckBunCheck, CheckClientBuild, CheckServerTest, CheckServerVet:
		return true
	default:
		return false
	}
}

func validateRelativePath(value string, allowEmpty bool) error {
	if value == "" && allowEmpty {
		return nil
	}
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || strings.ContainsAny(value, "\\:") || strings.HasPrefix(value, "/") {
		return ErrInvalid
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return ErrInvalid
		}
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") || (clean == "." && !allowEmpty) {
		return ErrInvalid
	}
	for _, segment := range strings.Split(strings.ToLower(clean), "/") {
		if secretPathSegment(segment) {
			return ErrInvalid
		}
	}
	return nil
}

func secretPathSegment(segment string) bool {
	if segment == ".env" || strings.HasPrefix(segment, ".env.") {
		return true
	}
	switch segment {
	case ".git", ".ssh", ".gnupg", ".aws", ".kube", ".docker", ".azure", ".netrc", ".npmrc", ".pypirc", ".pgpass",
		"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "credentials", "credential", "secrets", "secret":
		return true
	}
	if strings.Contains(segment, "credential") || strings.Contains(segment, "secret") || strings.Contains(segment, "password") {
		return true
	}
	switch path.Ext(segment) {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore", ".kdbx", ".ovpn":
		return true
	}
	return false
}

func secretQuery(query string) bool {
	query = strings.ToLower(query)
	return strings.Contains(query, ".env") || strings.Contains(query, "private key") || strings.Contains(query, "begin rsa") || strings.Contains(query, "/.ssh") || strings.Contains(query, "id_rsa")
}

func validateToolOutput(operation string, output ToolOutput) error {
	if operation == ToolListFiles {
		for _, file := range output.Files {
			if err := validateRelativePath(file, false); err != nil {
				return fmt.Errorf("worker returned a forbidden path")
			}
		}
	}
	if operation == ToolSearchCode {
		for _, match := range output.Matches {
			if match.Line < 1 || len(match.Text) > 10000 || validateRelativePath(match.Path, false) != nil {
				return fmt.Errorf("worker returned an invalid search result")
			}
		}
	}
	if operation == ToolCreateMergeRequest && output.OK && (output.MergeRequestIID < 1 || output.MergeRequestURL == "") {
		return fmt.Errorf("worker returned an incomplete merge request")
	}
	return nil
}
