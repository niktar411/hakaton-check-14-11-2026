// Package gitlabprotect enforces main-branch protection through the GitLab API.
package gitlabprotect

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// RuleName is the required approval rule.
const RuleName = "required-teamlead-approval"

// Config is the GitLab protection configuration.
type Config struct {
	APIURL    string
	Token     string
	Project   string
	AdminID   string
	AdminName string
	Branch    string
}

// Client calls the GitLab API with a personal access token.
type Client struct {
	config Config
	http   *http.Client
}

// New validates configuration and builds a client.
func New(config Config) (*Client, error) {
	if config.APIURL == "" || config.Token == "" || config.Project == "" {
		return nil, errors.New("GitLab API configuration is incomplete")
	}
	parsed, err := url.Parse(config.APIURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("GitLab API URL is invalid")
	}
	return &Client{
		config: Config{
			APIURL: strings.TrimRight(config.APIURL, "/"), Token: config.Token,
			Project: config.Project, AdminID: config.AdminID, AdminName: config.AdminName,
			Branch: config.Branch,
		},
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) call(method, path string, payload any, allowed ...int) (any, error) {
	if len(allowed) == 0 {
		allowed = []int{http.StatusOK, http.StatusCreated}
	}
	var body io.Reader
	if payload != nil {
		content, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(content)
	}
	request, err := http.NewRequest(method, c.config.APIURL+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("PRIVATE-TOKEN", c.config.Token)
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, errors.New("GitLab API is unavailable")
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil || len(content) > 2*1024*1024 {
		return nil, errors.New("GitLab API returned an invalid response")
	}
	permitted := false
	for _, status := range allowed {
		if response.StatusCode == status {
			permitted = true
			break
		}
	}
	if !permitted {
		return nil, fmt.Errorf("GitLab API rejected %s %s with status %d", method, path, response.StatusCode)
	}
	// Allowed non-2xx statuses (for example a missing protected branch) are
	// handled by the caller and carry no JSON document.
	if response.StatusCode >= 300 {
		return nil, nil
	}
	if len(content) == 0 {
		return nil, nil
	}
	var decoded any
	if err := json.Unmarshal(content, &decoded); err != nil {
		return nil, errors.New("GitLab API returned invalid JSON")
	}
	return decoded, nil
}

func (c *Client) projectPath() string {
	return "/projects/" + url.PathEscape(c.config.Project)
}

func (c *Client) administrator() (int64, error) {
	var users []any
	if c.config.AdminID != "" {
		id, err := strconv.ParseInt(c.config.AdminID, 10, 64)
		if err != nil {
			return 0, errors.New("GITLAB_ADMIN_ID must be numeric")
		}
		user, err := c.call(http.MethodGet, "/users/"+strconv.FormatInt(id, 10), nil)
		if err != nil {
			return 0, err
		}
		users = []any{user}
	} else if c.config.AdminName != "" {
		raw, err := c.call(http.MethodGet, "/users?username="+url.QueryEscape(c.config.AdminName), nil)
		if err != nil {
			return 0, err
		}
		var ok bool
		users, ok = raw.([]any)
		if !ok {
			return 0, errors.New("GitLab administrator lookup was not exact")
		}
	} else {
		return 0, errors.New("configure the GitLab administrator")
	}
	if len(users) != 1 {
		return 0, errors.New("GitLab administrator lookup was not exact")
	}
	user, ok := users[0].(map[string]any)
	if !ok {
		return 0, errors.New("GitLab administrator lookup was not exact")
	}
	id, ok := toInt(user["id"])
	if !ok || user["state"] != "active" || user["is_admin"] != true {
		return 0, errors.New("configured approver is not an active GitLab administrator")
	}
	return id, nil
}

func toInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed), true
		}
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	}
	return 0, false
}

// Enforce applies and verifies the protection policy.
func (c *Client) Enforce() error {
	if c.config.Branch != "main" {
		return errors.New("only main is supported by this protection policy")
	}
	adminID, err := c.administrator()
	if err != nil {
		return err
	}
	projectPath := c.projectPath()
	project, err := c.call(http.MethodGet, projectPath, nil)
	if err != nil {
		return err
	}
	if _, ok := project.(map[string]any); !ok {
		return errors.New("GitLab project response is invalid")
	}
	if _, err := c.call(http.MethodPatch, projectPath, map[string]any{
		"only_allow_merge_if_pipeline_succeeds":            true,
		"only_allow_merge_if_all_discussions_are_resolved": true,
		"allow_merge_on_skipped_pipeline":                  false,
		"reset_approvals_on_push":                          true,
		"merge_method":                                     "merge_commit",
		"remove_source_branch":                             true,
	}); err != nil {
		return err
	}
	encoded := url.PathEscape(c.config.Branch)
	protected, err := c.call(http.MethodGet, projectPath+"/protected_branches/"+encoded, nil,
		http.StatusOK, http.StatusNotFound)
	if err != nil {
		return err
	}
	policy := map[string]any{
		"allowed_to_push":      []any{map[string]any{"access_level": 0}},
		"allowed_to_merge":     []any{map[string]any{"user_id": adminID}},
		"allowed_to_unprotect": []any{map[string]any{"access_level": 0}},
		"allow_force_push":     false,
	}
	if protected == nil {
		protected, err = c.call(http.MethodPost, projectPath+"/protected_branches",
			withName(policy, c.config.Branch))
	} else {
		protected, err = c.call(http.MethodPatch, projectPath+"/protected_branches/"+encoded, policy)
	}
	if err != nil {
		return err
	}
	protectedObject, ok := protected.(map[string]any)
	if !ok {
		return errors.New("GitLab did not return the protected branch")
	}
	branchID, ok := toInt(protectedObject["id"])
	if !ok {
		return errors.New("GitLab did not return the protected branch")
	}

	rules, err := c.call(http.MethodGet, projectPath+"/approval_rules", nil)
	if err != nil {
		return err
	}
	ruleList, ok := rules.([]any)
	if !ok {
		return errors.New("GitLab approval rules are unavailable; a plan supporting required approvals is required")
	}
	approval := map[string]any{
		"name": RuleName, "approvals_required": 1,
		"user_ids": []any{adminID}, "group_ids": []any{},
		"protected_branch_ids": []any{branchID},
	}
	var existing map[string]any
	for _, item := range ruleList {
		rule, ok := item.(map[string]any)
		if ok && rule["name"] == RuleName {
			existing = rule
			break
		}
	}
	if existing != nil {
		id, _ := toInt(existing["id"])
		if _, err := c.call(http.MethodPut, projectPath+"/approval_rules/"+strconv.FormatInt(id, 10), approval); err != nil {
			return err
		}
	} else {
		if _, err := c.call(http.MethodPost, projectPath+"/approval_rules", approval); err != nil {
			return err
		}
	}

	verifiedProject, err := c.call(http.MethodGet, projectPath, nil)
	if err != nil {
		return err
	}
	projectObject, _ := verifiedProject.(map[string]any)
	if projectObject["only_allow_merge_if_pipeline_succeeds"] != true {
		return errors.New("GitLab pipeline requirement was not applied")
	}
	if projectObject["only_allow_merge_if_all_discussions_are_resolved"] != true {
		return errors.New("GitLab discussion requirement was not applied")
	}
	if projectObject["allow_merge_on_skipped_pipeline"] != false {
		return errors.New("GitLab must not merge skipped pipelines")
	}
	if projectObject["reset_approvals_on_push"] != true {
		return errors.New("GitLab must reset approvals after a new push")
	}
	verifiedBranch, err := c.call(http.MethodGet, projectPath+"/protected_branches/"+encoded, nil)
	if err != nil {
		return err
	}
	branchObject, _ := verifiedBranch.(map[string]any)
	if branchObject["allow_force_push"] != false {
		return errors.New("GitLab direct-push protection was not applied")
	}
	pushes, _ := branchObject["allowed_to_push"].([]any)
	if len(pushes) == 0 {
		return errors.New("GitLab direct-push protection was not applied")
	}
	for _, item := range pushes {
		entry, _ := item.(map[string]any)
		if level, ok := toInt(entry["access_level"]); !ok || level != 0 {
			return errors.New("GitLab direct-push protection was not applied")
		}
	}
	finalRules, err := c.call(http.MethodGet, projectPath+"/approval_rules", nil)
	if err != nil {
		return err
	}
	finalList, _ := finalRules.([]any)
	matching := []map[string]any{}
	for _, item := range finalList {
		rule, ok := item.(map[string]any)
		if ok && rule["name"] == RuleName {
			matching = append(matching, rule)
		}
	}
	if len(matching) != 1 {
		return errors.New("GitLab administrator approval was not applied")
	}
	if required, ok := toInt(matching[0]["approvals_required"]); !ok || required != 1 {
		return errors.New("GitLab administrator approval was not applied")
	}
	approvers, _ := matching[0]["users"].([]any)
	found := false
	for _, item := range approvers {
		user, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := toInt(user["id"]); ok && id == adminID {
			found = true
		}
	}
	if !found {
		return errors.New("GitLab approval rule does not target the administrator")
	}
	return nil
}

func withName(policy map[string]any, name string) map[string]any {
	result := map[string]any{"name": name}
	for key, value := range policy {
		result[key] = value
	}
	return result
}
