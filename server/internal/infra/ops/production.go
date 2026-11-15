// Package ops implements the fixed-command control agents.
package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ProductionError carries a safe failure code and the audit result for the API.
type ProductionError struct {
	Code   string
	Result map[string]any
}

func (e *ProductionError) Error() string { return e.Code }

var (
	releasePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	identifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
	backupPattern     = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

type releaseRecord struct {
	Release string `json:"release"`
	Backup  string `json:"backup"`
}

type pendingState struct {
	Release  string        `json:"release"`
	Previous releaseRecord `json:"previous"`
}

type releaseState struct {
	Current   releaseRecord  `json:"current"`
	Previous  *releaseRecord `json:"previous"`
	Pending   *pendingState  `json:"pending,omitempty"`
	LastError string         `json:"lastError,omitempty"`
}

// ComposeFunc runs one fixed docker compose invocation.
type ComposeFunc func(ctx context.Context, arguments []string, release string, stdout io.Writer, stdin io.Reader, timeout time.Duration) error

// Production performs transactional compose operations on SERVER 1.
type Production struct {
	ComposeFile    string
	EnvFile        string
	StateFile      string
	BackupRoot     string
	HealthURL      string
	DatabaseUser   string
	DatabaseName   string
	InitialRelease string

	Compose     ComposeFunc
	Healthy     func() bool
	WaitHealthy func() bool
}

// NewProduction validates absolute paths and the loopback health URL.
func NewProduction(composeFile, envFile, stateFile, backupRoot, healthURL, databaseUser, databaseName, initialRelease string) (*Production, error) {
	for _, path := range []string{composeFile, envFile, stateFile, backupRoot} {
		if !filepath.IsAbs(path) {
			return nil, errors.New("production paths must be absolute")
		}
	}
	if !identifierPattern.MatchString(databaseUser) || !identifierPattern.MatchString(databaseName) {
		return nil, errors.New("invalid production database identifier")
	}
	if initialRelease != "" && !releasePattern.MatchString(initialRelease) {
		return nil, errors.New("invalid initial release")
	}
	if !strings.HasPrefix(healthURL, "http://127.0.0.1") && !strings.HasPrefix(healthURL, "http://localhost") &&
		!strings.HasPrefix(healthURL, "https://127.0.0.1") && !strings.HasPrefix(healthURL, "https://localhost") {
		return nil, errors.New("production health URL must be loopback")
	}
	return &Production{
		ComposeFile: composeFile, EnvFile: envFile, StateFile: stateFile, BackupRoot: backupRoot,
		HealthURL: healthURL, DatabaseUser: databaseUser, DatabaseName: databaseName,
		InitialRelease: initialRelease,
	}, nil
}

// ProductionFromEnv reads the production agent configuration.
func ProductionFromEnv() (*Production, error) {
	return NewProduction(
		envOr("PRODUCTION_COMPOSE_FILE", "/opt/hackathon/docker-compose.yml"),
		envOr("PRODUCTION_ENV_FILE", "/etc/hackathon/main.env"),
		envOr("PRODUCTION_STATE", "/var/lib/hackathon-agent/release.json"),
		envOr("PRODUCTION_BACKUP_ROOT", "/var/backups/hackathon"),
		envOr("PRODUCTION_HEALTH_URL", "http://127.0.0.1/health"),
		envOr("PRODUCTION_DB_USER", "postgres"),
		envOr("PRODUCTION_DB_NAME", "hackathon"),
		os.Getenv("PRODUCTION_RELEASE"),
	)
}

func (p *Production) compose() ComposeFunc {
	if p.Compose != nil {
		return p.Compose
	}
	return p.runCompose
}

func (p *Production) runCompose(ctx context.Context, arguments []string, release string, stdout io.Writer, stdin io.Reader, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append([]string{
		"compose", "--project-name", "hackathon", "--env-file", p.EnvFile,
		"-f", p.ComposeFile,
	}, arguments...)
	command := exec.CommandContext(ctx, "docker", args...)
	command.Env = append(os.Environ(), "RELEASE="+release)
	command.Stdout = stdout
	command.Stderr = os.Stderr
	command.Stdin = stdin
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("compose timeout")
		}
		return errors.New("compose command failed")
	}
	return nil
}

func (p *Production) healthy() bool {
	if p.Healthy != nil {
		return p.Healthy()
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: noRedirect}
	response, err := client.Get(p.HealthURL)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1025))
	return err == nil && len(body) <= 1024
}

func (p *Production) waitHealthy(ctx context.Context) bool {
	if p.WaitHealthy != nil {
		return p.WaitHealthy()
	}
	for attempt := 0; attempt < 30; attempt++ {
		if p.healthy() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(time.Second):
		}
	}
	return false
}

func (p *Production) atomicState(state releaseState) error {
	if err := os.MkdirAll(filepath.Dir(p.StateFile), 0o700); err != nil {
		return err
	}
	content, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return atomicWrite(p.StateFile, content, 0o600)
}

func (p *Production) state() (releaseState, error) {
	content, err := os.ReadFile(p.StateFile)
	if err == nil {
		var state releaseState
		if err := json.Unmarshal(content, &state); err != nil || !releasePattern.MatchString(state.Current.Release) {
			return releaseState{}, &ProductionError{Code: "release_state_invalid"}
		}
		return state, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return releaseState{}, &ProductionError{Code: "release_state_invalid"}
	}
	if p.InitialRelease == "" {
		return releaseState{}, &ProductionError{Code: "release_state_unconfigured"}
	}
	state := releaseState{Current: releaseRecord{Release: p.InitialRelease}}
	if err := p.atomicState(state); err != nil {
		return releaseState{}, &ProductionError{Code: "release_state_invalid"}
	}
	return state, nil
}

func (p *Production) backup(ctx context.Context, release, action string) (string, error) {
	if !releasePattern.MatchString(release) {
		return "", &ProductionError{Code: "invalid_release"}
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000Z")
	name := fmt.Sprintf("%s-%s-%s-%s", stamp, release, action, randomHex(3))
	destination := filepath.Join(p.BackupRoot, name)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return "", &ProductionError{Code: "backup_failed"}
	}
	_ = os.Chmod(destination, 0o700)
	dump, err := os.OpenFile(filepath.Join(destination, "postgres.dump"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		os.RemoveAll(destination)
		return "", &ProductionError{Code: "backup_failed"}
	}
	err = p.compose()(ctx, []string{
		"exec", "-T", "postgres", "pg_dump", "--format=custom", "--no-owner",
		"--no-privileges", "-U", p.DatabaseUser, "-d", p.DatabaseName,
	}, release, dump, nil, 55*time.Second)
	closeErr := dump.Close()
	if err != nil || closeErr != nil {
		os.RemoveAll(destination)
		return "", &ProductionError{Code: "compose_command_failed"}
	}
	if err := copyFile(p.ComposeFile, filepath.Join(destination, "docker-compose.yml")); err != nil {
		os.RemoveAll(destination)
		return "", &ProductionError{Code: "backup_failed"}
	}
	if _, err := os.Stat(p.StateFile); err == nil {
		if err := copyFile(p.StateFile, filepath.Join(destination, "release.json")); err != nil {
			os.RemoveAll(destination)
			return "", &ProductionError{Code: "backup_failed"}
		}
	}
	metadata, _ := json.Marshal(map[string]string{
		"release": release, "action": action, "createdAt": time.Now().UTC().Format(time.RFC3339),
	})
	if err := os.WriteFile(filepath.Join(destination, "metadata.json"), metadata, 0o600); err != nil {
		os.RemoveAll(destination)
		return "", &ProductionError{Code: "backup_failed"}
	}
	return name, nil
}

func (p *Production) restoreDatabase(ctx context.Context, backupName string) error {
	if !backupPattern.MatchString(backupName) {
		return &ProductionError{Code: "backup_invalid"}
	}
	dumpPath := filepath.Join(p.BackupRoot, backupName, "postgres.dump")
	info, err := os.Lstat(dumpPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return &ProductionError{Code: "backup_not_found"}
	}
	dump, err := os.Open(dumpPath)
	if err != nil {
		return &ProductionError{Code: "backup_not_found"}
	}
	defer dump.Close()
	state, err := p.state()
	if err != nil {
		return err
	}
	if err := p.compose()(ctx, []string{
		"exec", "-T", "postgres", "pg_restore", "--clean", "--if-exists", "--no-owner",
		"--no-privileges", "-U", p.DatabaseUser, "-d", p.DatabaseName,
	}, state.Current.Release, nil, dump, 55*time.Second); err != nil {
		return &ProductionError{Code: "compose_command_failed"}
	}
	return nil
}

func (p *Production) restore(ctx context.Context, release, backupName string) bool {
	if err := p.compose()(ctx, []string{"stop", "--timeout", "15", "api", "web", "orchestrator"}, release, nil, nil, 45*time.Second); err != nil {
		return false
	}
	if err := p.restoreDatabase(ctx, backupName); err != nil {
		return false
	}
	if err := p.compose()(ctx, []string{"up", "--detach", "--no-build", "api", "web", "orchestrator"}, release, nil, nil, 45*time.Second); err != nil {
		return false
	}
	return p.waitHealthy(ctx)
}

func (p *Production) failure(code, status, backup string) *ProductionError {
	return &ProductionError{Code: code, Result: map[string]any{
		"status": status, "backup_name": backup, "details": map[string]any{"error": code},
	}}
}

// Run executes one production operation and never uses a shell.
func (p *Production) Run(ctx context.Context, action, release string) (map[string]any, error) {
	if action != "deploy" && action != "rollback" && action != "restart" && action != "start" && action != "stop" {
		return nil, &ProductionError{Code: "invalid_operation"}
	}
	release = strings.TrimSpace(release)
	if release != "" && !releasePattern.MatchString(release) {
		return nil, &ProductionError{Code: "invalid_release"}
	}
	state, err := p.state()
	if err != nil {
		return nil, err
	}
	current := state.Current.Release
	if action == "deploy" && (release == "" || release == current) {
		return nil, &ProductionError{Code: "new_release_required"}
	}
	backupName, err := p.backup(ctx, current, action)
	if err != nil {
		return nil, err
	}
	old := releaseRecord{Release: current, Backup: backupName}
	state.Current = old
	if err := p.atomicState(state); err != nil {
		return nil, &ProductionError{Code: "release_state_invalid"}
	}
	switch action {
	case "deploy":
		return p.deploy(ctx, state, old, release, backupName)
	case "rollback":
		return p.rollback(ctx, state, old, release, backupName)
	case "start", "restart":
		arguments := []string{"up", "--detach", "--no-build"}
		if action == "restart" {
			arguments = append(arguments, "--force-recreate")
		}
		arguments = append(arguments, "api", "web", "orchestrator")
		if err := p.compose()(ctx, arguments, current, nil, nil, 45*time.Second); err != nil {
			return p.rollbackFromError(ctx, action, current, backupName, &ProductionError{Code: "compose_command_failed"})
		}
		if !p.waitHealthy(ctx) {
			restored := p.restore(ctx, current, backupName)
			if !restored {
				return nil, p.failure("rollback_failed", "rollback_failed", backupName)
			}
			return nil, p.failure("health_check_failed", "rolled_back", backupName)
		}
		if err := p.atomicState(state); err != nil {
			return nil, &ProductionError{Code: "release_state_invalid"}
		}
		return map[string]any{"status": "completed", "backup_name": backupName,
			"details": map[string]any{"release": current}}, nil
	default:
		if err := p.compose()(ctx, []string{"stop", "--timeout", "20", "api", "web", "orchestrator"}, current, nil, nil, 45*time.Second); err != nil {
			return nil, &ProductionError{Code: "compose_command_failed"}
		}
		return map[string]any{"status": "completed", "backup_name": backupName,
			"details": map[string]any{"release": current}}, nil
	}
}

func (p *Production) rollbackFromError(ctx context.Context, action, current, backupName string, cause *ProductionError) (map[string]any, error) {
	if action == "start" || action == "restart" {
		if p.restore(ctx, current, backupName) {
			return nil, p.failure(cause.Code, "rolled_back", backupName)
		}
		return nil, p.failure("rollback_failed", "rollback_failed", backupName)
	}
	return nil, cause
}

func (p *Production) deploy(ctx context.Context, state releaseState, old releaseRecord, release, backupName string) (map[string]any, error) {
	previous := state.Previous
	state.Pending = &pendingState{Release: release, Previous: old}
	if err := p.atomicState(state); err != nil {
		return nil, &ProductionError{Code: "release_state_invalid"}
	}
	fail := func(code string) (map[string]any, error) {
		if p.restore(ctx, old.Release, backupName) {
			restored := releaseState{Current: old, Previous: previous, LastError: code}
			_ = p.atomicState(restored)
			return nil, p.failure(code, "rolled_back", backupName)
		}
		broken := releaseState{Current: old, Previous: previous, Pending: &pendingState{Release: release}, LastError: "rollback_failed"}
		_ = p.atomicState(broken)
		return nil, p.failure("rollback_failed", "rollback_failed", backupName)
	}
	if err := p.compose()(ctx, []string{"pull", "--ignore-pull-failures", "api", "web"}, release, nil, nil, 45*time.Second); err != nil {
		return fail("compose_command_failed")
	}
	if err := p.compose()(ctx, []string{"up", "--detach", "--no-build", "api", "web", "orchestrator"}, release, nil, nil, 45*time.Second); err != nil {
		return fail("compose_command_failed")
	}
	if !p.waitHealthy(ctx) {
		return fail("health_check_failed")
	}
	final := releaseState{Current: releaseRecord{Release: release}, Previous: &old}
	if err := p.atomicState(final); err != nil {
		return nil, &ProductionError{Code: "release_state_invalid"}
	}
	return map[string]any{"status": "completed", "backup_name": backupName,
		"details": map[string]any{"release": release}}, nil
}

func (p *Production) rollback(ctx context.Context, state releaseState, old releaseRecord, release, backupName string) (map[string]any, error) {
	previous := state.Previous
	if previous == nil {
		return nil, &ProductionError{Code: "previous_release_required"}
	}
	target := release
	if target == "" {
		target = previous.Release
	}
	if target == old.Release {
		target = previous.Release
	}
	targetBackup := previous.Backup
	if target != previous.Release {
		targetBackup = ""
	}
	if !releasePattern.MatchString(target) {
		return nil, &ProductionError{Code: "invalid_release"}
	}
	if targetBackup != "" {
		if !p.restore(ctx, target, targetBackup) {
			return nil, p.failure("rollback_failed", "rollback_failed", backupName)
		}
	} else {
		if err := p.compose()(ctx, []string{"up", "--detach", "--no-build", "api", "web", "orchestrator"}, target, nil, nil, 45*time.Second); err != nil {
			return nil, p.failure("rollback_failed", "rollback_failed", backupName)
		}
		if !p.waitHealthy(ctx) {
			return nil, p.failure("rollback_failed", "rollback_failed", backupName)
		}
	}
	final := releaseState{Current: releaseRecord{Release: target, Backup: targetBackup}, Previous: &old}
	if err := p.atomicState(final); err != nil {
		return nil, &ProductionError{Code: "release_state_invalid"}
	}
	return map[string]any{"status": "completed", "backup_name": backupName,
		"details": map[string]any{"release": target}}, nil
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
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

func copyFile(source, target string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, content, 0o600)
}
