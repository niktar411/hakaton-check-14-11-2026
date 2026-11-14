package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// ToolPrepareWorkspace creates the isolated repository copy.
	ToolPrepareWorkspace = "prepare_workspace"
	// ToolListFiles lists repository files.
	ToolListFiles = "list_files"
	// ToolReadFile reads one repository file.
	ToolReadFile = "read_file"
	// ToolSearchCode searches repository text.
	ToolSearchCode = "search_code"
	// ToolWriteFile writes one repository file.
	ToolWriteFile = "write_file"
	// ToolRunCheck runs one fixed verification.
	ToolRunCheck = "run_check"
	// ToolCreateMergeRequest commits, pushes and opens a merge request.
	ToolCreateMergeRequest = "create_merge_request"
)

// Checks are the only verifications the worker can run.
var Checks = map[string]bool{
	"bun-check": true, "client-build": true, "server-test": true, "server-vet": true,
}

// Tools are the only model-visible worker operations.
var Tools = map[string]bool{
	ToolPrepareWorkspace: true, ToolListFiles: true, ToolReadFile: true,
	ToolSearchCode: true, ToolWriteFile: true, ToolRunCheck: true,
	ToolCreateMergeRequest: true,
}

// DeploymentActions are the worker-side container operations.
var DeploymentActions = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"create_test_database": true, "drop_test_database": true,
}

// Config controls the restricted worker.
type Config struct {
	Workspaces     string
	Reviews        string
	State          string
	RepositoryURL  string
	GitLabAPIURL   string
	GitLabProject  string
	GitLabToken    string
	CodingImage    string
	PostgresImage  string
	BaseBranch     string
	ReviewURL      string
	ReviewBindIP   string
	GitAskPass     string
	CheckTimeout   int
	ComposeFile    string
	ComposeEnv     string
	MaxConcurrency int
}

// ConfigFromEnv reads the worker configuration.
func ConfigFromEnv() Config {
	root := envOr("WORKER_STATE_ROOT", "/var/lib/hackathon-worker")
	return Config{
		Workspaces:     envOr("WORKER_WORKSPACES", filepath.Join(root, "workspaces")),
		Reviews:        envOr("WORKER_REVIEWS", filepath.Join(root, "reviews")),
		State:          envOr("WORKER_STATE", filepath.Join(root, "state")),
		RepositoryURL:  os.Getenv("WORKER_REPOSITORY_URL"),
		GitLabAPIURL:   os.Getenv("GITLAB_API_URL"),
		GitLabProject:  os.Getenv("GITLAB_PROJECT"),
		GitLabToken:    os.Getenv("GITLAB_TOKEN"),
		CodingImage:    envOr("WORKER_CODING_IMAGE", "hackathon-worker-coding:local"),
		PostgresImage:  envOr("WORKER_POSTGRES_IMAGE", "postgres:18.1-alpine3.22"),
		BaseBranch:     envOr("WORKER_BASE_BRANCH", "main"),
		ReviewURL:      envOr("WORKER_REVIEW_URL", "http://10.77.0.10:8080"),
		ReviewBindIP:   envOr("WORKER_BIND_IP", "10.77.0.10"),
		GitAskPass:     envOr("WORKER_GIT_ASKPASS", "/usr/local/bin/hackathon-git-askpass"),
		CheckTimeout:   envInt("WORKER_CHECK_TIMEOUT", 105),
		ComposeFile:    envOr("WORKER_COMPOSE_FILE", "/opt/hackathon/docker-compose.worker.yml"),
		ComposeEnv:     envOr("WORKER_COMPOSE_ENV_FILE", "/etc/hackathon/worker.env"),
		MaxConcurrency: envInt("WORKER_MAX_CONCURRENCY", 2),
	}
}

// Validate rejects unsafe worker configuration.
func (c Config) Validate() error {
	if _, err := ImageReference(c.CodingImage); err != nil {
		return err
	}
	if _, err := ImageReference(c.PostgresImage); err != nil {
		return err
	}
	if _, err := BranchName(c.BaseBranch); err != nil {
		return err
	}
	if c.CheckTimeout < 10 || c.CheckTimeout > 110 || c.MaxConcurrency < 1 || c.MaxConcurrency > 4 {
		return errors.New("worker limits are outside their safe range")
	}
	parsed, err := url.Parse(c.ReviewURL)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != c.ReviewBindIP ||
		(parsed.Path != "" && parsed.Path != "/") {
		return errors.New("review URL must use only the worker VPN address")
	}
	return nil
}

// Worker owns the fixed tool implementations.
type Worker struct {
	config   Config
	mu       sync.Mutex
	locks    map[int64]*sync.Mutex
	slots    chan struct{}
	resource sync.Mutex
	client   *http.Client
}

// NewWorker validates configuration and builds a worker.
func NewWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Worker{
		config: config,
		locks:  map[int64]*sync.Mutex{},
		slots:  make(chan struct{}, config.MaxConcurrency),
		client: &http.Client{
			Timeout: 35 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// Bootstrap creates the fixed directories and starts the coding network.
func (w *Worker) Bootstrap() error {
	if err := os.MkdirAll(w.config.Workspaces, 0o700); err != nil {
		return err
	}
	// The review container runs as the unprivileged nginx user, which is not
	// the worker identity, so published review artifacts are group/world
	// readable while the port stays bound to the VPN address.
	if err := os.MkdirAll(w.config.Reviews, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(w.config.Reviews, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(w.config.State, 0o700); err != nil {
		return err
	}
	return w.codingAction("start")
}

func (w *Worker) workspace(id int64) string {
	return filepath.Join(w.config.Workspaces, fmt.Sprintf("task-%d", id))
}

func (w *Worker) stateFile(id int64) string {
	return filepath.Join(w.config.State, fmt.Sprintf("task-%d.json", id))
}

type workspaceState struct {
	TaskID       int64  `json:"taskId"`
	ParentTaskID int64  `json:"parentTaskId"`
	Branch       string `json:"branch"`
	BaseBranch   string `json:"baseBranch"`
	CreatedAt    string `json:"createdAt"`
}

func (w *Worker) loadState(id int64) (workspaceState, error) {
	content, err := os.ReadFile(w.stateFile(id))
	if err != nil {
		return workspaceState{}, workerError("workspace_not_found", 404)
	}
	var state workspaceState
	if err := json.Unmarshal(content, &state); err != nil {
		return workspaceState{}, workerError("workspace_not_found", 404)
	}
	if state.TaskID != id || state.Branch != fmt.Sprintf("coding/task-%d", id) {
		return workspaceState{}, workerError("workspace_state_invalid", 503)
	}
	return state, nil
}

func (w *Worker) writeState(id int64, state workspaceState) error {
	if err := os.MkdirAll(w.config.State, 0o700); err != nil {
		return err
	}
	content, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return atomicWrite(w.stateFile(id), content, 0o600)
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".new"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(temporary)
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(temporary)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temporary)
		return err
	}
	return os.Rename(temporary, path)
}

// Execute runs one fixed worker tool for one task.
func (w *Worker) Execute(operation string, body map[string]any) (map[string]any, error) {
	if !Tools[operation] {
		return nil, workerError("invalid_operation", 400)
	}
	id, err := TaskID(body["taskId"])
	if err != nil {
		return nil, err
	}
	select {
	case w.slots <- struct{}{}:
	case <-time.After(time.Second):
		return nil, workerError("worker_busy", 503)
	}
	lock := w.taskLock(id)
	if !lock.TryLock() {
		<-w.slots
		return nil, workerError("task_busy", 409)
	}
	defer func() {
		lock.Unlock()
		<-w.slots
	}()
	result, err := w.dispatch(operation, id, body)
	if err != nil {
		return nil, err
	}
	result["ok"] = true
	return result, nil
}

func (w *Worker) taskLock(id int64) *sync.Mutex {
	w.mu.Lock()
	defer w.mu.Unlock()
	lock, ok := w.locks[id]
	if !ok {
		lock = &sync.Mutex{}
		w.locks[id] = lock
	}
	return lock
}

func (w *Worker) dispatch(operation string, id int64, body map[string]any) (map[string]any, error) {
	switch operation {
	case ToolPrepareWorkspace:
		return w.prepareWorkspace(id, body)
	case ToolListFiles:
		return w.listFiles(id, body)
	case ToolReadFile:
		return w.readFile(id, body)
	case ToolSearchCode:
		return w.searchCode(id, body)
	case ToolWriteFile:
		return w.writeFile(id, body)
	case ToolRunCheck:
		return w.runCheck(id, body)
	case ToolCreateMergeRequest:
		return w.createMergeRequest(id, body)
	default:
		return nil, workerError("invalid_operation", 400)
	}
}

func (w *Worker) prepareWorkspace(id int64, body map[string]any) (map[string]any, error) {
	parent := int64(0)
	if raw, ok := body["parentTaskId"]; ok && raw != nil && raw != "" {
		parsed, err := TaskID(raw)
		if err != nil {
			return nil, err
		}
		parent = parsed
	}
	if parent == id {
		return nil, workerError("invalid_parent_task", 400)
	}
	workspace := w.workspace(id)
	if _, err := os.Stat(workspace); err == nil {
		if _, err := w.loadState(id); err != nil {
			return nil, err
		}
		return map[string]any{"output": "workspace ready", "content": fmt.Sprintf("coding/task-%d", id)}, nil
	}
	if err := os.MkdirAll(w.config.Workspaces, 0o700); err != nil {
		return nil, workerError("workspace_unavailable", 503)
	}
	if err := os.MkdirAll(w.config.State, 0o700); err != nil {
		return nil, workerError("workspace_unavailable", 503)
	}
	source := w.config.RepositoryURL
	parsed, err := url.Parse(source)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Hostname() == "" || parsed.User != nil {
		return nil, workerError("repository_not_configured", 503)
	}
	base := w.config.BaseBranch
	if parent != 0 {
		if _, err := w.loadState(parent); err != nil {
			return nil, err
		}
		source = w.workspace(parent)
		base = fmt.Sprintf("coding/task-%d", parent)
	}
	git := w.gitOptions(time.Duration(w.config.CheckTimeout) * time.Second)
	git.Timeout = 90 * time.Second
	if _, err := Run([]string{"git", "clone", "--no-tags", "--single-branch", "--branch", base, "--", source, workspace}, git); err != nil {
		os.RemoveAll(workspace)
		return nil, err
	}
	branch := fmt.Sprintf("coding/task-%d", id)
	git.Dir = workspace
	if _, err := Run([]string{"git", "switch", "-c", branch}, git); err != nil {
		os.RemoveAll(workspace)
		os.Remove(w.stateFile(id))
		return nil, err
	}
	state := workspaceState{
		TaskID: id, ParentTaskID: parent, Branch: branch, BaseBranch: base,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := w.writeState(id, state); err != nil {
		os.RemoveAll(workspace)
		return nil, workerError("workspace_unavailable", 503)
	}
	return map[string]any{"output": "workspace prepared", "content": branch}, nil
}

func (w *Worker) listFiles(id int64, body map[string]any) (map[string]any, error) {
	workspace := w.workspace(id)
	if _, err := w.loadState(id); err != nil {
		return nil, err
	}
	path, _ := body["path"].(string)
	root, err := SafePath(workspace, path, true, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, workerError("invalid_path", 400)
	}
	files := make([]string, 0, 64)
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if SensitiveName(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if relative, relErr := filepath.Rel(workspace, current); relErr == nil {
			files = append(files, filepath.ToSlash(relative))
			if len(files) >= 500 {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return nil, workerError("workspace_unavailable", 503)
	}
	result := map[string]any{"files": files}
	if len(files) >= 500 {
		result["output"] = "file limit reached"
	}
	return result, nil
}

func (w *Worker) readFile(id int64, body map[string]any) (map[string]any, error) {
	workspace := w.workspace(id)
	if _, err := w.loadState(id); err != nil {
		return nil, err
	}
	path, _ := body["path"].(string)
	target, err := SafePath(workspace, path, false, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return nil, workerError("path_not_found", 404)
	}
	if info.Size() > FileReadLimit {
		return nil, workerError("file_too_large", 413)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return nil, workerError("file_not_readable", 413)
	}
	if len(content) > FileReadLimit || strings.ContainsRune(string(content), 0) {
		return nil, workerError("file_not_readable", 413)
	}
	return map[string]any{"content": string(content)}, nil
}

func (w *Worker) searchCode(id int64, body map[string]any) (map[string]any, error) {
	workspace := w.workspace(id)
	if _, err := w.loadState(id); err != nil {
		return nil, err
	}
	path, _ := body["path"].(string)
	root, err := SafePath(workspace, path, true, false)
	if err != nil {
		return nil, err
	}
	query, _ := body["query"].(string)
	query, err = RequireText(query, "query", 1000, false)
	if err != nil {
		return nil, err
	}
	if query != strings.TrimSpace(query) {
		return nil, workerError("invalid_query", 400)
	}
	lowered := strings.ToLower(query)
	for _, token := range []string{".env", "private key", "begin rsa", "/.ssh", "id_rsa"} {
		if strings.Contains(lowered, token) {
			return nil, workerError("invalid_query", 400)
		}
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, workerError("invalid_path", 400)
	}
	matches := make([]map[string]any, 0, 16)
	err = filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if SensitiveName(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			if entry.IsDir() && entry.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil || info.Size() > 1024*1024 {
			return nil
		}
		found, done := searchFile(workspace, current, query)
		matches = append(matches, found...)
		if done || len(matches) >= 100 {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, workerError("workspace_unavailable", 503)
	}
	result := map[string]any{"matches": matches}
	if len(matches) >= 100 {
		result["output"] = "match limit reached"
	}
	return result, nil
}

// searchFile returns matches for one regular repository file.
func searchFile(workspace, current, query string) ([]map[string]any, bool) {
	content, readErr := os.ReadFile(current)
	if readErr != nil {
		return nil, false
	}
	text := string(content)
	if strings.ContainsRune(text, 0) {
		return nil, false
	}
	relative, relErr := filepath.Rel(workspace, current)
	if relErr != nil {
		return nil, false
	}
	found := []map[string]any{}
	for index, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, query) {
			continue
		}
		if len(line) > 2000 {
			line = line[:2000]
		}
		found = append(found, map[string]any{
			"path": filepath.ToSlash(relative), "line": index + 1, "text": line,
		})
		if len(found) >= 100 {
			return found, true
		}
	}
	return found, false
}

func (w *Worker) writeFile(id int64, body map[string]any) (map[string]any, error) {
	workspace := w.workspace(id)
	if _, err := w.loadState(id); err != nil {
		return nil, err
	}
	path, _ := body["path"].(string)
	target, err := SafePath(workspace, path, false, true)
	if err != nil {
		return nil, err
	}
	content, ok := body["content"].(string)
	if !ok || strings.ContainsRune(content, 0) || len([]byte(content)) > FileWriteLimit {
		return nil, workerError("invalid_content", 400)
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		if !info.Mode().IsRegular() {
			return nil, workerError("invalid_path", 400)
		}
		mode = info.Mode().Perm()
	}
	temporary := filepath.Join(filepath.Dir(target), fmt.Sprintf(".worker-write-%d", time.Now().UnixNano()))
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return nil, workerError("write_failed", 503)
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		os.Remove(temporary)
		return nil, workerError("write_failed", 503)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(temporary)
		return nil, workerError("write_failed", 503)
	}
	if err := file.Close(); err != nil {
		os.Remove(temporary)
		return nil, workerError("write_failed", 503)
	}
	if err := os.Rename(temporary, target); err != nil {
		os.Remove(temporary)
		return nil, workerError("write_failed", 503)
	}
	return map[string]any{"output": "file written"}, nil
}

func (w *Worker) dockerCleanup(name string) {
	if !namePattern.MatchString(name) {
		return
	}
	_, _ = RunBounded([]string{"docker", "rm", "--force", name}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024})
}

func (w *Worker) networkExists(name string) bool {
	result, err := RunBounded([]string{"docker", "network", "inspect", "--format", "{{.Name}}", name},
		ProcessOptions{Timeout: 5 * time.Second, OutputLimit: 1024})
	return err == nil && result.ReturnCode == 0
}

func (w *Worker) volumeExists(name string) bool {
	result, err := RunBounded([]string{"docker", "volume", "inspect", "--format", "{{.Name}}", name},
		ProcessOptions{Timeout: 5 * time.Second, OutputLimit: 1024})
	return err == nil && result.ReturnCode == 0
}

func (w *Worker) createTaskDatabase(id int64) (string, error) {
	name := fmt.Sprintf("worker-db-task-%d", id)
	password := randomToken(32)
	w.dockerCleanup(name)
	if w.networkExists(name) {
		_, _ = Run([]string{"docker", "network", "rm", name}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024})
	}
	if _, err := Run([]string{"docker", "network", "create", "--internal", name}, ProcessOptions{Timeout: 15 * time.Second, OutputLimit: 1024}); err != nil {
		return "", err
	}
	if _, err := Run([]string{"docker", "volume", "create", name}, ProcessOptions{Timeout: 15 * time.Second, OutputLimit: 1024}); err != nil {
		return "", err
	}
	database := fmt.Sprintf("task_%d_test", id)
	if _, err := Run([]string{
		"docker", "run", "--detach", "--name", name, "--network", name,
		"--network-alias", name, "--restart", "no", "--pids-limit", "128",
		"--memory", "512m", "--cpus", "1",
		"--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE",
		"--cap-add", "FOWNER", "--cap-add", "SETGID", "--cap-add", "SETUID",
		"--security-opt", "no-new-privileges",
		"--mount", "type=volume,src=" + name + ",dst=/var/lib/postgresql",
		"--env", "POSTGRES_USER=worker", "--env", "POSTGRES_PASSWORD=" + password,
		"--env", "POSTGRES_DB=" + database, w.config.PostgresImage,
	}, ProcessOptions{Timeout: 30 * time.Second, OutputLimit: 1024}); err != nil {
		w.dropTaskDatabase(id)
		return "", err
	}
	for attempt := 0; attempt < 30; attempt++ {
		result, err := RunBounded([]string{"docker", "exec", name, "pg_isready", "-U", "worker", "-d", database},
			ProcessOptions{Timeout: 3 * time.Second, OutputLimit: 1024})
		if err == nil && result.ReturnCode == 0 {
			return password, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	w.dropTaskDatabase(id)
	return "", workerError("database_not_ready", 503)
}

func (w *Worker) dropTaskDatabase(id int64) {
	name := fmt.Sprintf("worker-db-task-%d", id)
	w.dockerCleanup(name)
	if w.networkExists(name) {
		_, _ = Run([]string{"docker", "network", "rm", name}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024})
	}
	if w.volumeExists(name) {
		_, _ = Run([]string{"docker", "volume", "rm", name}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024})
	}
}

func (w *Worker) runCheck(id int64, body map[string]any) (map[string]any, error) {
	check, _ := body["check"].(string)
	if !Checks[check] {
		return nil, workerError("invalid_check", 400)
	}
	workspace := w.workspace(id)
	if _, err := w.loadState(id); err != nil {
		return nil, err
	}
	if !w.networkExists("hackathon-coding") {
		return nil, workerError("coding_stopped", 409)
	}
	name := fmt.Sprintf("worker-check-task-%d-%s", id, check)
	review := filepath.Join(w.config.Reviews, fmt.Sprintf("task-%d", id))
	if check == "client-build" {
		if err := os.MkdirAll(review, 0o755); err != nil {
			return nil, workerError("review_unavailable", 503)
		}
		_ = os.Chmod(review, 0o755)
	}
	password := ""
	network := "none"
	if check == "server-test" {
		var err error
		password, err = w.createTaskDatabase(id)
		if err != nil {
			return nil, err
		}
		network = fmt.Sprintf("worker-db-task-%d", id)
	}
	args := []string{
		"docker", "run", "--rm", "--name", name, "--network", network,
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "256", "--memory", "2g", "--cpus", "2",
		"--user", containerUser(), "--workdir", "/work/repository",
		"--tmpfs", "/work:rw,nosuid,nodev,size=1536m,mode=1777",
		"--tmpfs", "/tmp:rw,nosuid,nodev,size=512m,mode=1777",
		"--mount", "type=bind,src=" + workspace + ",dst=/source,readonly",
	}
	if check == "client-build" {
		args = append(args, "--mount", "type=bind,src="+review+",dst=/output")
	}
	if check == "server-test" {
		database := fmt.Sprintf("task_%d_test", id)
		connection := fmt.Sprintf("postgres://worker:%s@worker-db-task-%d:5432/%s?sslmode=disable",
			url.QueryEscape(password), id, database)
		args = append(args, "--env", "TEST_DATABASE_URL="+connection, "--env", "DATABASE_URL="+connection)
	}
	args = append(args, w.config.CodingImage, "/opt/check-runner", check)
	w.dockerCleanup(name)
	result, err := RunBounded(args, ProcessOptions{
		Timeout: time.Duration(w.config.CheckTimeout) * time.Second, OutputLimit: 64 * 1024,
	})
	w.dockerCleanup(name)
	if check == "server-test" {
		w.dropTaskDatabase(id)
	}
	if err != nil {
		return nil, err
	}
	if result.ReturnCode >= 125 {
		return nil, workerError("check_infrastructure_failed", 503)
	}
	output := result.Output
	if result.Truncated {
		output += "\n[output truncated]"
	}
	if check == "client-build" && result.ReturnCode == 0 {
		output += fmt.Sprintf("\nReview: %s/task-%d/", strings.TrimRight(w.config.ReviewURL, "/"), id)
	}
	return map[string]any{"passed": result.ReturnCode == 0, "output": output}, nil
}

func (w *Worker) changedPaths(workspace string) ([]string, error) {
	options := w.gitOptions(30 * time.Second)
	options.Dir = workspace
	result, err := Run([]string{"git", "status", "--porcelain=v1", "-z", "--untracked-files=all"}, options)
	if err != nil {
		return nil, err
	}
	entries := strings.Split(result.Output, "\x00")
	changed := make([]string, 0, 8)
	for index := 0; index < len(entries); index++ {
		entry := entries[index]
		if entry == "" {
			continue
		}
		if len(entry) < 4 {
			return nil, workerError("git_state_invalid", 503)
		}
		status, value := entry[:2], entry[3:]
		if strings.ContainsAny(status, "RC") {
			index++
			if index >= len(entries) || entries[index] == "" {
				return nil, workerError("git_state_invalid", 503)
			}
		}
		relative, err := RelativePath(value, false)
		if err != nil {
			return nil, err
		}
		changed = append(changed, relative)
	}
	if len(changed) == 0 {
		return nil, workerError("no_changes", 409)
	}
	return changed, nil
}

func (w *Worker) createMergeRequest(id int64, body map[string]any) (map[string]any, error) {
	workspace := w.workspace(id)
	state, err := w.loadState(id)
	if err != nil {
		return nil, err
	}
	title, _ := body["title"].(string)
	title, err = RequireText(title, "title", 200, false)
	if err != nil {
		return nil, err
	}
	description, _ := body["description"].(string)
	if len(description) > 16000 || strings.ContainsRune(description, 0) {
		return nil, workerError("invalid_description", 400)
	}
	if w.config.GitLabToken == "" || w.config.GitLabProject == "" {
		return nil, workerError("gitlab_not_configured", 503)
	}
	changed, err := w.changedPaths(workspace)
	if err != nil {
		return nil, err
	}
	options := w.gitOptions(60 * time.Second)
	options.Dir = workspace
	for start := 0; start < len(changed); start += 100 {
		end := start + 100
		if end > len(changed) {
			end = len(changed)
		}
		args := append([]string{"git", "add", "--"}, changed[start:end]...)
		if _, err := Run(args, options); err != nil {
			return nil, err
		}
	}
	if _, err := Run([]string{
		"git", "-c", "user.name=Hackathon Worker", "-c", "user.email=hackathon-worker@localhost",
		"commit", "-m", title,
	}, options); err != nil {
		return nil, err
	}
	branch := fmt.Sprintf("coding/task-%d", id)
	if _, err := Run([]string{"git", "push", "--set-upstream", "origin", "HEAD:refs/heads/" + branch}, options); err != nil {
		return nil, err
	}
	base := state.BaseBranch
	if strings.HasPrefix(base, "coding/task-") {
		base = w.config.BaseBranch
	}
	payload, err := json.Marshal(map[string]any{
		"source_branch": branch, "target_branch": base, "title": title,
		"description": description, "remove_source_branch": true,
	})
	if err != nil {
		return nil, workerError("merge_request_failed", 503)
	}
	endpoint := strings.TrimRight(w.config.GitLabAPIURL, "/") + "/projects/" +
		url.PathEscape(w.config.GitLabProject) + "/merge_requests"
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return nil, workerError("merge_request_failed", 503)
	}
	request.Header.Set("PRIVATE-TOKEN", w.config.GitLabToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := w.client.Do(request)
	if err != nil {
		return nil, workerError("merge_request_failed", 503)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return nil, workerError("merge_request_failed", 503)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, ResponseLimit+1))
	if err != nil {
		return nil, workerError("merge_request_failed", 503)
	}
	if len(content) > ResponseLimit {
		return nil, workerError("merge_request_response_too_large", 503)
	}
	var decoded struct {
		IID    int64  `json:"iid"`
		WebURL string `json:"web_url"`
	}
	if err := json.Unmarshal(content, &decoded); err != nil || decoded.IID < 1 {
		return nil, workerError("merge_request_response_invalid", 503)
	}
	expected, err := url.Parse(w.config.GitLabAPIURL)
	if err != nil {
		return nil, workerError("gitlab_not_configured", 503)
	}
	actual, err := url.Parse(decoded.WebURL)
	if err != nil || actual.Scheme != expected.Scheme || actual.Hostname() != expected.Hostname() ||
		actual.User != nil {
		return nil, workerError("merge_request_response_invalid", 503)
	}
	return map[string]any{
		"mergeRequestIid": decoded.IID, "mergeRequestUrl": decoded.WebURL, "output": "merge request created",
	}, nil
}

func (w *Worker) compose(action, target string) error {
	base := []string{
		"docker", "compose", "--project-name", "hackathon-worker", "--env-file",
		w.config.ComposeEnv, "-f", w.config.ComposeFile,
	}
	var args []string
	switch action {
	case "start":
		args = append(base, "up", "--detach", "--no-build", "--no-deps", target)
	case "stop":
		args = append(base, "stop", "--timeout", "20", target)
	default:
		args = append(base, "restart", "--timeout", "20", target)
	}
	_, err := Run(args, ProcessOptions{Timeout: 60 * time.Second, OutputLimit: 64 * 1024})
	return err
}

func (w *Worker) codingAction(action string) error {
	if action == "stop" || action == "restart" {
		result, err := Run([]string{"docker", "ps", "--filter", "name=^/worker-check-task-", "--format", "{{.Names}}"},
			ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 64 * 1024})
		if err != nil {
			return err
		}
		names := []string{}
		for _, line := range strings.Split(result.Output, "\n") {
			name := strings.TrimSpace(line)
			if namePattern.MatchString(name) && strings.HasPrefix(name, "worker-check-task-") {
				names = append(names, name)
			}
			if len(names) == 16 {
				break
			}
		}
		for _, name := range names {
			_, _ = Run([]string{"docker", "stop", "--time", "5", name}, ProcessOptions{Timeout: 15 * time.Second, OutputLimit: 1024})
		}
		if w.networkExists("hackathon-coding") {
			_, _ = Run([]string{"docker", "network", "rm", "hackathon-coding"}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024})
		}
	}
	if (action == "start" || action == "restart") && !w.networkExists("hackathon-coding") {
		if _, err := Run([]string{"docker", "network", "create", "--internal", "hackathon-coding"}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024}); err != nil {
			return err
		}
	}
	_, err := Run([]string{"docker", "image", "inspect", w.config.CodingImage}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024})
	return err
}

func (w *Worker) sharedDatabase(create bool) error {
	name := "worker-test-database"
	if create {
		if !w.networkExists(name) {
			if _, err := Run([]string{"docker", "network", "create", "--internal", name}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024}); err != nil {
				return err
			}
		}
		if _, err := Run([]string{"docker", "volume", "create", name}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024}); err != nil {
			return err
		}
		exists, err := RunBounded([]string{"docker", "container", "inspect", name}, ProcessOptions{Timeout: 5 * time.Second, OutputLimit: 1024})
		if err == nil && exists.ReturnCode == 0 {
			_, err = Run([]string{"docker", "start", name}, ProcessOptions{Timeout: 20 * time.Second, OutputLimit: 1024})
			return err
		}
		password := randomToken(32)
		_, err = Run([]string{
			"docker", "run", "--detach", "--name", name, "--network", name,
			"--restart", "unless-stopped", "--pids-limit", "128", "--memory", "512m",
			"--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE",
			"--cap-add", "FOWNER", "--cap-add", "SETGID", "--cap-add", "SETUID",
			"--security-opt", "no-new-privileges",
			"--mount", "type=volume,src=" + name + ",dst=/var/lib/postgresql",
			"--env", "POSTGRES_USER=worker", "--env", "POSTGRES_PASSWORD=" + password,
			"--env", "POSTGRES_DB=worker_test", w.config.PostgresImage,
		}, ProcessOptions{Timeout: 30 * time.Second, OutputLimit: 1024})
		return err
	}
	w.dockerCleanup(name)
	if w.networkExists(name) {
		_, _ = Run([]string{"docker", "network", "rm", name}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024})
	}
	if w.volumeExists(name) {
		_, _ = Run([]string{"docker", "volume", "rm", name}, ProcessOptions{Timeout: 10 * time.Second, OutputLimit: 1024})
	}
	return nil
}

// Deployment runs one fixed container or temporary database operation.
func (w *Worker) Deployment(action, target string) (map[string]any, error) {
	if !DeploymentActions[action] || (target != "coding" && target != "review" && target != "test") {
		return nil, workerError("invalid_operation", 400)
	}
	if target == "test" && action != "create_test_database" && action != "drop_test_database" {
		return nil, workerError("invalid_operation", 400)
	}
	if target != "test" && action != "start" && action != "stop" && action != "restart" {
		return nil, workerError("invalid_operation", 400)
	}
	if !w.resource.TryLock() {
		return nil, workerError("operation_in_progress", 409)
	}
	defer w.resource.Unlock()
	var err error
	switch target {
	case "coding":
		err = w.codingAction(action)
	case "review":
		err = w.compose(action, "review")
	default:
		err = w.sharedDatabase(action == "create_test_database")
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": "completed", "backup_name": "", "details": map[string]any{"target": target}}, nil
}

func (w *Worker) gitOptions(timeout time.Duration) ProcessOptions {
	environment := append([]string{}, os.Environ()...)
	environment = append(environment,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS="+w.config.GitAskPass,
		"GITLAB_TOKEN="+w.config.GitLabToken,
		"GITLAB_USERNAME="+envOr("GITLAB_USERNAME", "oauth2"),
	)
	return ProcessOptions{Env: environment, Timeout: timeout, OutputLimit: 64 * 1024}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
