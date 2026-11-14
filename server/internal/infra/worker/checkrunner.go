package worker

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// CheckPaths are the fixed mounts of a disposable check container.
type CheckPaths struct {
	Source       string
	Root         string
	Output       string
	Dependencies string
}

// DefaultCheckPaths matches the Docker mounts created by Worker.runCheck.
func DefaultCheckPaths() CheckPaths {
	return CheckPaths{
		Source: "/source", Root: "/work/repository", Output: "/output",
		Dependencies: "/opt/dependencies/node_modules",
	}
}

// RunCheck is the entry point of the prebuilt, disposable check container.
func RunCheck(check string) int {
	return runCheck(check, DefaultCheckPaths())
}

func runCheck(check string, paths CheckPaths) int {
	if !Checks[check] {
		return 2
	}
	if err := prepare(paths); err != nil {
		fmt.Fprintln(os.Stderr, "isolated check failed:", err)
		return 2
	}
	switch check {
	case "bun-check":
		return run(paths.Root, "bun", "run", "check")
	case "client-build":
		if code := run(paths.Root, "bun", "run", "build:client"); code != 0 {
			return code
		}
		if err := publish(paths); err != nil {
			fmt.Fprintln(os.Stderr, "publish failed:", err)
			return 2
		}
		return 0
	case "server-test":
		if len(os.Getenv("TEST_DATABASE_URL")) < len("postgres://worker:") ||
			os.Getenv("TEST_DATABASE_URL")[:len("postgres://worker:")] != "postgres://worker:" {
			return 2
		}
		return run(filepath.Join(paths.Root, "server"), "go", "test", "./...")
	case "server-vet":
		return run(filepath.Join(paths.Root, "server"), "go", "vet", "./...")
	}
	return 2
}

func ignoredDirectoryEntry(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "__pycache__":
		return true
	}
	return SensitiveName(name)
}

func prepare(paths CheckPaths) error {
	if err := copyTree(paths.Source, paths.Root, ignoredDirectoryEntry); err != nil {
		return err
	}
	if info, err := os.Stat(paths.Dependencies); err == nil && info.IsDir() {
		target := filepath.Join(paths.Root, "node_modules")
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			if err := os.Symlink(paths.Dependencies, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyTree(source, target string, ignore func(string) bool) error {
	sourceInfo, err := os.Lstat(source)
	if err != nil || !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source is not a directory")
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, current)
		if err != nil || relative == "." {
			return nil
		}
		if ignore(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		return copyFile(current, destination)
	})
}

func copyFile(source, target string) error {
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}

func publish(paths CheckPaths) error {
	artifact := filepath.Join(paths.Root, "dist")
	info, err := os.Lstat(artifact)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("client build did not produce a regular dist directory")
	}
	if err := os.MkdirAll(paths.Output, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(paths.Output)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(paths.Output, entry.Name())); err != nil {
			return err
		}
	}
	if err := copyTree(artifact, paths.Output, ignoredDirectoryEntry); err != nil {
		return err
	}
	return filepath.WalkDir(paths.Output, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if SensitiveName(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("unsafe review artifact")
		}
		return nil
	})
}

func run(directory string, name string, args ...string) int {
	command := exec.Command(name, args...)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"CI=1", "GOCACHE=/tmp/go-build", "GOMODCACHE=/opt/gopath/pkg/mod",
		"GOPATH=/opt/gopath", "GOFLAGS=-mod=readonly", "CGO_ENABLED=0",
		"HOME=/tmp", "TMPDIR=/tmp", "XDG_CACHE_HOME=/tmp",
		"BUN_INSTALL_CACHE_DIR=/tmp/bun-cache",
	)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError.ExitCode()
		}
		return 2
	}
	return 0
}
