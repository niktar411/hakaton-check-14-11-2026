package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckRunnerRejectsUnknownChecks(t *testing.T) {
	if code := runCheck("python-test", DefaultCheckPaths()); code != 2 {
		t.Fatalf("unknown check returned %d", code)
	}
}

func TestIgnoredEntries(t *testing.T) {
	for _, name := range []string{".git", "node_modules", "dist", "__pycache__", ".env", "id_rsa", "server.pem"} {
		if !ignoredDirectoryEntry(name) {
			t.Fatalf("%q is not ignored", name)
		}
	}
	for _, name := range []string{"src", "main.go", "package.json"} {
		if ignoredDirectoryEntry(name) {
			t.Fatalf("%q is ignored", name)
		}
	}
}

func TestPublishCopiesOnlyRegularArtifacts(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	root := filepath.Join(directory, "root")
	output := filepath.Join(directory, "output")
	if err := os.MkdirAll(filepath.Join(source, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "assets", "app.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(source, root, ignoredDirectoryEntry); err != nil {
		t.Fatal(err)
	}
	// The client build writes dist/ inside the copied workspace.
	if err := os.MkdirAll(filepath.Join(root, "dist", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "assets", "app.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := CheckPaths{Root: root, Output: output, Source: source}
	if err := publish(paths); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(output, "index.html")); err != nil || string(content) != "ok" {
		t.Fatalf("published index = %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(output, "assets", "app.js")); err != nil {
		t.Fatal(err)
	}
}
