package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	workerErr, ok := err.(Error)
	if !ok || workerErr.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func TestRelativePathRejectsEscapesAndSecrets(t *testing.T) {
	invalid := []string{
		"/etc/passwd", `\windows\system32`, "C:/Users/secret", "../outside",
		"a/../../b", "", "./a", "a\\b", ".env", ".env.production",
		"config/.env", "secrets/token.txt", "deploy/id_rsa",
		"deploy/id_ed25519.pub", "certs/server.pem", "credentials.json",
		".git/config", ".config/gcloud/application_default_credentials.json",
	}
	for _, value := range invalid {
		if _, err := RelativePath(value, false); err == nil {
			t.Fatalf("RelativePath(%q) accepted", value)
		}
	}
	for _, value := range []string{"client/src/main.tsx", "server/README.md", "a-b_c/d.e"} {
		if got, err := RelativePath(value, false); err != nil || got != value {
			t.Fatalf("RelativePath(%q) = %q, %v", value, got, err)
		}
	}
	if got, err := RelativePath("", true); err != nil || got != "" {
		t.Fatalf("empty path = %q, %v", got, err)
	}
}

func TestSafePathRejectsEscapingSymlink(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, "workspace")
	outside := filepath.Join(directory, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "data.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks are unavailable in this environment")
	}
	_, err := SafePath(root, "link/data.txt", false, false)
	requireCode(t, err, "symlink_forbidden")
}

func TestSafePathReportsMissingPaths(t *testing.T) {
	directory := t.TempDir()
	missing := filepath.Join(directory, "missing")
	_, err := SafePath(missing, "file", false, false)
	requireCode(t, err, "workspace_not_found")

	existing := filepath.Join(directory, "workspace")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = SafePath(existing, "no/such/file", false, false)
	requireCode(t, err, "path_not_found")

	target, err := SafePath(existing, "a/b/file.txt", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(target)) != "b" {
		t.Fatalf("target = %s", target)
	}
}

func TestTaskIDIsBounded(t *testing.T) {
	if value, err := TaskID(float64(42)); err != nil || value != 42 {
		t.Fatalf("TaskID(42) = %d, %v", value, err)
	}
	if value, err := TaskID("7"); err != nil || value != 7 {
		t.Fatalf("TaskID(7) = %d, %v", value, err)
	}
	for _, value := range []any{true, 0, -1, TaskIDMax + 1, "1e3", 1.5, nil} {
		if _, err := TaskID(value); err == nil {
			t.Fatalf("TaskID(%v) accepted", value)
		}
	}
}

func TestBranchSlugAndImageValidation(t *testing.T) {
	for _, value := range []string{"-main", "/main", "main/", "main.", "main.lock", "a..b", "a@{b", "a//b", "main\n"} {
		if _, err := BranchName(value); err == nil {
			t.Fatalf("BranchName(%q) accepted", value)
		}
	}
	if value, err := BranchName("coding/task-7"); err != nil || value != "coding/task-7" {
		t.Fatalf("BranchName = %q, %v", value, err)
	}
	if value, err := Slug("client"); err != nil || value != "client" {
		t.Fatalf("Slug = %q, %v", value, err)
	}
	for _, value := range []string{"Client", "a b", "-a"} {
		if _, err := Slug(value); err == nil {
			t.Fatalf("Slug(%q) accepted", value)
		}
	}
	if _, err := ImageReference("registry.local/team/image:1.2"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"bad image", "image\n", "", "-bad"} {
		if _, err := ImageReference(value); err == nil {
			t.Fatalf("ImageReference(%q) accepted", value)
		}
	}
}

func TestRequireTextRejectsEmptyOversizedAndNul(t *testing.T) {
	if _, err := RequireText("value", "name", 10, false); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "  ", "abcdefghijk", "a\x00b"} {
		if _, err := RequireText(value, "name", 10, false); err == nil {
			t.Fatalf("RequireText(%q) accepted", value)
		}
	}
}
