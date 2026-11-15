package ops

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordedCall struct {
	arguments []string
	release   string
}

type recorder struct {
	calls []recordedCall
	fail  func(arguments []string) error
}

func (r *recorder) run(_ context.Context, arguments []string, release string, _ io.Writer, _ io.Reader, _ time.Duration) error {
	r.calls = append(r.calls, recordedCall{arguments: append([]string{}, arguments...), release: release})
	if r.fail != nil {
		return r.fail(arguments)
	}
	return nil
}

func newProduction(t *testing.T, initial string) (*Production, *recorder) {
	t.Helper()
	directory := t.TempDir()
	composeFile := filepath.Join(directory, "docker-compose.yml")
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(directory, "main.env")
	if err := os.WriteFile(envFile, []byte("RELEASE=release-a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	production, err := NewProduction(
		composeFile, envFile, filepath.Join(directory, "state", "release.json"),
		filepath.Join(directory, "backups"), "http://127.0.0.1/health",
		"postgres", "hackathon", initial,
	)
	if err != nil {
		t.Fatal(err)
	}
	compose := &recorder{}
	production.Compose = compose.run
	return production, compose
}

func productionError(t *testing.T, err error) *ProductionError {
	t.Helper()
	var target *ProductionError
	if !errors.As(err, &target) {
		t.Fatalf("error = %v, want ProductionError", err)
	}
	return target
}

func TestHealthURLMustBeLoopback(t *testing.T) {
	directory := t.TempDir()
	_, err := NewProduction(
		filepath.Join(directory, "compose.yml"), filepath.Join(directory, "main.env"),
		filepath.Join(directory, "state.json"), filepath.Join(directory, "backups"),
		"https://portal.example/health", "postgres", "hackathon", "release-a",
	)
	if err == nil {
		t.Fatal("external health URL accepted")
	}
}

func TestSuccessfulDeployBacksUpBeforeSwitching(t *testing.T) {
	production, compose := newProduction(t, "release-a")
	production.WaitHealthy = func() bool { return true }
	result, err := production.Run(context.Background(), "deploy", "release-b")
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != "completed" || result["backup_name"] == "" {
		t.Fatalf("result = %v", result)
	}
	first := strings.Join(compose.calls[0].arguments, " ")
	if !strings.HasPrefix(first, "exec -T postgres pg_dump") {
		t.Fatalf("first compose call = %s", first)
	}
	backup := filepath.Join(production.BackupRoot, result["backup_name"].(string))
	for _, name := range []string{"postgres.dump", "docker-compose.yml", "metadata.json"} {
		if _, err := os.Stat(filepath.Join(backup, name)); err != nil {
			t.Fatalf("backup %s missing: %v", name, err)
		}
	}
	state := readState(t, production.StateFile)
	if state.Current.Release != "release-b" || state.Previous == nil || state.Previous.Release != "release-a" {
		t.Fatalf("state = %+v", state)
	}
}

func TestUnhealthyDeployRollsBack(t *testing.T) {
	production, compose := newProduction(t, "release-a")
	calls := 0
	production.WaitHealthy = func() bool {
		calls++
		return calls > 1
	}
	_, err := production.Run(context.Background(), "deploy", "release-b")
	problem := productionError(t, err)
	if problem.Code != "health_check_failed" || problem.Result["status"] != "rolled_back" {
		t.Fatalf("error = %+v", problem)
	}
	state := readState(t, production.StateFile)
	if state.Current.Release != "release-a" {
		t.Fatalf("state = %+v", state)
	}
	found := false
	for _, call := range compose.calls {
		if strings.Contains(strings.Join(call.arguments, " "), "pg_restore") {
			found = true
		}
	}
	if !found {
		t.Fatal("database was not restored")
	}
}

func TestSameReleaseCannotBeRedeployed(t *testing.T) {
	production, _ := newProduction(t, "release-a")
	_, err := production.Run(context.Background(), "deploy", "release-a")
	if code := productionError(t, err).Code; code != "new_release_required" {
		t.Fatalf("code = %s", code)
	}
	if entries, err := os.ReadDir(production.BackupRoot); err == nil && len(entries) != 0 {
		t.Fatalf("backups were created: %v", entries)
	}
}

func TestUnsafeReleaseNamesAreRejected(t *testing.T) {
	production, _ := newProduction(t, "release-a")
	for _, value := range []string{"../escape", "release=b", "release b", "-flag", "release/../../x"} {
		_, err := production.Run(context.Background(), "rollback", value)
		if code := productionError(t, err).Code; code != "invalid_release" {
			t.Fatalf("release %q code = %s", value, code)
		}
	}
}

func TestRollbackRequiresPreviousRelease(t *testing.T) {
	production, _ := newProduction(t, "release-a")
	production.WaitHealthy = func() bool { return true }
	_, err := production.Run(context.Background(), "rollback", "")
	if code := productionError(t, err).Code; code != "previous_release_required" {
		t.Fatalf("code = %s", code)
	}
}

func TestFailedBackupLeavesNoEvidence(t *testing.T) {
	production, compose := newProduction(t, "release-a")
	production.WaitHealthy = func() bool { return true }
	compose.fail = func(arguments []string) error {
		for _, argument := range arguments {
			if argument == "pg_dump" {
				return errors.New("compose failed")
			}
		}
		return nil
	}
	if _, err := production.Run(context.Background(), "deploy", "release-b"); err == nil {
		t.Fatal("failed backup accepted")
	}
	if entries, err := os.ReadDir(production.BackupRoot); err == nil && len(entries) != 0 {
		t.Fatalf("partial backup left behind: %v", entries)
	}
	state := readState(t, production.StateFile)
	if state.Current.Release != "release-a" || state.Pending != nil {
		t.Fatalf("state = %+v", state)
	}
}

func TestRestoreRejectsUnsafeBackupNames(t *testing.T) {
	production, _ := newProduction(t, "release-a")
	err := production.restoreDatabase(context.Background(), "../etc")
	if code := productionError(t, err).Code; code != "backup_invalid" {
		t.Fatalf("code = %s", code)
	}
}

func readState(t *testing.T, path string) releaseState {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state releaseState
	if err := jsonUnmarshal(content, &state); err != nil {
		t.Fatal(err)
	}
	return state
}
