package gitlab

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ProviderGitLab = "gitlab"
	ProviderGitea  = "gitea"
)

type Config struct {
	URL      string
	Token    string
	Project  string
	Provider string
}

type Pipeline struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	SHA    string `json:"sha"`
	WebURL string `json:"web_url"`
}

type MergeRequest struct {
	IID          int64    `json:"iid"`
	State        string   `json:"state"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	SHA          string   `json:"sha"`
	WebURL       string   `json:"web_url"`
	HeadPipeline Pipeline `json:"head_pipeline"`
}

type Approver struct {
	User struct {
		Username string `json:"username"`
	} `json:"user"`
}

type Approvals struct {
	Approved      bool       `json:"approved"`
	ApprovalsLeft int        `json:"approvals_left"`
	ApprovedBy    []Approver `json:"approved_by"`
}

type Client struct {
	config Config
	http   *http.Client
}

func NewClient(config Config, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if config.Provider == "" {
		config.Provider = ProviderGitLab
	}
	clone := *client
	clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{config: config, http: &clone}
}

func (c *Client) isGitea() bool {
	return c.config.Provider == ProviderGitea
}

func escapeProject(project string) string {
	parts := strings.Split(project, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

type TreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

type giteaTreeResponse struct {
	Tree      []TreeEntry `json:"tree"`
	Truncated bool        `json:"truncated"`
}

type giteaContentResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	Path     string `json:"path"`
	Type     string `json:"type"`
}

func escapeRepoPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// DefaultBranch returns the repository default branch. Gitea is the only
// provider used by the production deployment; GitLab falls back to main.
func (c *Client) DefaultBranch(ctx context.Context) (string, error) {
	if !c.isGitea() {
		return "main", nil
	}
	var repository struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.request(ctx, http.MethodGet, "", nil, &repository); err != nil {
		return "", fmt.Errorf("load repository: %w", err)
	}
	if strings.TrimSpace(repository.DefaultBranch) == "" {
		return "main", nil
	}
	return repository.DefaultBranch, nil
}

// Tree returns a bounded recursive file tree for the chat project snapshot.
func (c *Client) Tree(ctx context.Context, ref string) ([]TreeEntry, error) {
	if !c.isGitea() {
		return nil, fmt.Errorf("repository tree is not supported for this provider")
	}
	var result giteaTreeResponse
	path := "/git/trees/" + url.PathEscape(ref) + "?recursive=true&per_page=1000"
	if err := c.request(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, fmt.Errorf("load repository tree: %w", err)
	}
	return result.Tree, nil
}

// File reads one repository file. Callers must apply secret-path filtering and
// their own size limits before placing the content into a model prompt.
func (c *Client) File(ctx context.Context, path, ref string) ([]byte, error) {
	if !c.isGitea() {
		return nil, fmt.Errorf("repository file is not supported for this provider")
	}
	var result giteaContentResponse
	endpoint := "/contents/" + escapeRepoPath(path) + "?ref=" + url.QueryEscape(ref)
	if err := c.request(ctx, http.MethodGet, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf("load repository file: %w", err)
	}
	if result.Encoding != "base64" {
		return nil, fmt.Errorf("repository file has unsupported encoding")
	}
	content, err := base64.StdEncoding.DecodeString(result.Content)
	if err != nil {
		return nil, fmt.Errorf("decode repository file: %w", err)
	}
	return content, nil
}

func (c *Client) endpoint(path string) string {
	base := strings.TrimRight(c.config.URL, "/")
	if c.isGitea() {
		return base + "/api/v1/repos/" + escapeProject(c.config.Project) + path
	}
	return base + "/api/v4/projects/" + url.PathEscape(c.config.Project) + path
}

func (c *Client) request(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode git request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), body)
	if err != nil {
		return fmt.Errorf("create git request: %w", err)
	}
	if c.isGitea() {
		req.Header.Set("Authorization", "token "+c.config.Token)
	} else {
		req.Header.Set("PRIVATE-TOKEN", c.config.Token)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call git provider: %w", err)
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			slog.WarnContext(ctx, "close git response failed", "error", closeErr)
		}
	}()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("git provider returned status %d", res.StatusCode)
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(output); err != nil {
		return fmt.Errorf("decode git response: %w", err)
	}
	return nil
}

type giteaRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type giteaPullRequest struct {
	Number         int64    `json:"number"`
	State          string   `json:"state"`
	Merged         bool     `json:"merged"`
	HTMLURL        string   `json:"html_url"`
	Head           giteaRef `json:"head"`
	Base           giteaRef `json:"base"`
	MergeCommitSHA string   `json:"merge_commit_sha"`
}

type giteaCombinedStatus struct {
	State      string `json:"state"`
	TotalCount int    `json:"total_count"`
}

type giteaReview struct {
	State string `json:"state"`
	User  struct {
		Login string `json:"login"`
	} `json:"user"`
}

// Gitea uses "open" for a pull request, while the planner and webhook logic
// share the GitLab spelling "opened".
func normalizeGiteaState(state string, merged bool) string {
	if merged {
		return "merged"
	}
	if state == "open" {
		return "opened"
	}
	return state
}

func (c *Client) MergeRequestByIID(ctx context.Context, iid int64) (MergeRequest, error) {
	if c.isGitea() {
		return c.giteaMergeRequest(ctx, iid)
	}
	var result MergeRequest
	if err := c.request(ctx, http.MethodGet, "/merge_requests/"+strconv.FormatInt(iid, 10), nil, &result); err != nil {
		return MergeRequest{}, err
	}
	if err := c.validateMergeRequest(result); err != nil {
		return MergeRequest{}, err
	}
	return result, nil
}

func (c *Client) giteaMergeRequest(ctx context.Context, iid int64) (MergeRequest, error) {
	var pull giteaPullRequest
	if err := c.request(ctx, http.MethodGet, "/pulls/"+strconv.FormatInt(iid, 10), nil, &pull); err != nil {
		return MergeRequest{}, err
	}
	result := MergeRequest{
		IID:          pull.Number,
		State:        normalizeGiteaState(pull.State, pull.Merged),
		SourceBranch: pull.Head.Ref,
		TargetBranch: pull.Base.Ref,
		SHA:          pull.Head.SHA,
		WebURL:       pull.HTMLURL,
	}
	if status, err := c.giteaPipelineStatus(ctx, pull.Head.SHA); err == nil {
		result.HeadPipeline = Pipeline{Status: status, SHA: pull.Head.SHA}
	}
	if err := c.validateMergeRequest(result); err != nil {
		return MergeRequest{}, err
	}
	return result, nil
}

func (c *Client) giteaPipelineStatus(ctx context.Context, sha string) (string, error) {
	if sha == "" {
		return "", nil
	}
	var status giteaCombinedStatus
	if err := c.request(ctx, http.MethodGet, "/commits/"+url.PathEscape(sha)+"/status", nil, &status); err != nil {
		return "", err
	}
	if status.TotalCount == 0 && strings.EqualFold(status.State, "pending") {
		return "", nil
	}
	switch strings.ToLower(status.State) {
	case "failure", "error":
		return "failed", nil
	case "success", "pending":
		return strings.ToLower(status.State), nil
	default:
		return strings.ToLower(status.State), nil
	}
}

func (c *Client) validateMergeRequest(result MergeRequest) error {
	if result.WebURL == "" {
		return nil
	}
	base, err := url.Parse(c.config.URL)
	if err != nil {
		return fmt.Errorf("parse configured git URL: %w", err)
	}
	web, err := url.Parse(result.WebURL)
	if err != nil || web.User != nil || !strings.EqualFold(web.Host, base.Host) || web.Scheme != base.Scheme {
		return fmt.Errorf("merge request URL is outside configured git provider")
	}
	return nil
}

func (c *Client) MergeRequestApprovals(ctx context.Context, iid int64) (Approvals, error) {
	if c.isGitea() {
		return c.giteaApprovals(ctx, iid)
	}
	var result Approvals
	if err := c.request(ctx, http.MethodGet, "/merge_requests/"+strconv.FormatInt(iid, 10)+"/approvals", nil, &result); err != nil {
		return Approvals{}, err
	}
	return result, nil
}

func (c *Client) giteaApprovals(ctx context.Context, iid int64) (Approvals, error) {
	var reviews []giteaReview
	if err := c.request(ctx, http.MethodGet, "/pulls/"+strconv.FormatInt(iid, 10)+"/reviews", nil, &reviews); err != nil {
		return Approvals{}, err
	}
	result := Approvals{Approved: false, ApprovalsLeft: 1}
	for _, review := range reviews {
		if !strings.EqualFold(review.State, "APPROVED") || review.User.Login == "" {
			continue
		}
		result.Approved = true
		result.ApprovalsLeft = 0
		var approver Approver
		approver.User.Username = review.User.Login
		result.ApprovedBy = append(result.ApprovedBy, approver)
	}
	return result, nil
}

func (c *Client) ApproveMergeRequest(ctx context.Context, iid int64, sha string) error {
	if c.isGitea() {
		pull, err := c.MergeRequestByIID(ctx, iid)
		if err != nil {
			return err
		}
		if sha != "" && pull.SHA != sha {
			return fmt.Errorf("merge request head changed")
		}
		return c.request(ctx, http.MethodPost, "/pulls/"+strconv.FormatInt(iid, 10)+"/reviews", map[string]string{"event": "APPROVED", "body": "Approved via team portal"}, nil)
	}
	return c.request(ctx, http.MethodPost, "/merge_requests/"+strconv.FormatInt(iid, 10)+"/approve", map[string]string{"sha": sha}, nil)
}

func (c *Client) Merge(ctx context.Context, iid int64, sha string) error {
	if c.isGitea() {
		pull, err := c.MergeRequestByIID(ctx, iid)
		if err != nil {
			return err
		}
		if sha != "" && pull.SHA != sha {
			return fmt.Errorf("merge request head changed")
		}
		return c.request(ctx, http.MethodPost, "/pulls/"+strconv.FormatInt(iid, 10)+"/merge", map[string]any{
			"Do":                     "merge",
			"DeleteBranchAfterMerge": true,
			"MergeTitleField":        fmt.Sprintf("Merge pull request #%d", iid),
		}, nil)
	}
	return c.request(ctx, http.MethodPut, "/merge_requests/"+strconv.FormatInt(iid, 10)+"/merge", map[string]any{"sha": sha, "should_remove_source_branch": true, "merge_when_pipeline_succeeds": false}, nil)
}

func (c *Client) CreateMergeRequest(ctx context.Context, sourceBranch, title, description string) (MergeRequest, error) {
	if c.isGitea() {
		var pull giteaPullRequest
		if err := c.request(ctx, http.MethodPost, "/pulls", map[string]string{
			"head":  sourceBranch,
			"base":  "main",
			"title": title,
			"body":  description,
		}, &pull); err != nil {
			return MergeRequest{}, err
		}
		result := MergeRequest{
			IID:          pull.Number,
			State:        normalizeGiteaState(pull.State, pull.Merged),
			SourceBranch: pull.Head.Ref,
			TargetBranch: pull.Base.Ref,
			SHA:          pull.Head.SHA,
			WebURL:       pull.HTMLURL,
		}
		if err := c.validateMergeRequest(result); err != nil {
			return MergeRequest{}, err
		}
		return result, nil
	}
	var result MergeRequest
	input := map[string]string{"source_branch": sourceBranch, "target_branch": "main", "title": title, "description": description, "remove_source_branch": "true"}
	if err := c.request(ctx, http.MethodPost, "/merge_requests", input, &result); err != nil {
		return MergeRequest{}, err
	}
	if err := c.validateMergeRequest(result); err != nil {
		return MergeRequest{}, err
	}
	return result, nil
}
