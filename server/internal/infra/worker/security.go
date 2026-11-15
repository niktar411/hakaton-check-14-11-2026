// Package worker implements the restricted SERVER 2 coding worker.
package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	// TaskIDMax bounds every workspace identifier.
	TaskIDMax = 9999
	// FileReadLimit is the largest file the model may read.
	FileReadLimit = 64 * 1024
	// FileWriteLimit is the largest file the model may write.
	FileWriteLimit = 256 * 1024
	// ResponseLimit bounds every JSON response of the worker.
	ResponseLimit = 128 * 1024
)

// Error is safe to return to the authenticated backend.
type Error struct {
	Code   string
	Status int
}

func (e Error) Error() string { return e.Code }

func workerError(code string, status int) Error { return Error{Code: code, Status: status} }

var (
	slugPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	branchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$`)
	drivePattern  = regexp.MustCompile(`^[A-Za-z]:`)
	imagePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./:@-]{0,255}$`)
)

var secretDirectories = map[string]bool{
	".git": true, ".ssh": true, ".gnupg": true, ".aws": true, ".azure": true,
	".docker": true, ".kube": true, ".secrets": true, "secrets": true,
	"credentials": true, "private-keys": true,
}

var secretFiles = map[string]bool{
	".netrc": true, ".git-credentials": true, ".pgpass": true, ".npmrc": true,
	".pypirc": true, "authorized_keys": true, "known_hosts": true,
	"credentials": true, "credentials.json": true, "secrets.json": true,
	"secrets.yaml": true, "secrets.yml": true, "auth.json": true,
	"tokens.json": true, "kubeconfig": true,
}

var secretPaths = map[string]bool{".config/gcloud": true}

var secretSuffixes = []string{".key", ".pem", ".p12", ".pfx", ".kdbx", ".keystore", ".jks", ".ovpn"}

// SensitiveName reports whether a single path component must never be exposed.
func SensitiveName(name string) bool {
	lowered := strings.ToLower(name)
	if secretDirectories[lowered] {
		return true
	}
	if secretFiles[lowered] || strings.HasPrefix(lowered, ".env") {
		return true
	}
	for _, suffix := range secretSuffixes {
		if strings.HasSuffix(lowered, suffix) {
			return true
		}
	}
	if strings.HasPrefix(lowered, "id_rsa") || strings.HasPrefix(lowered, "id_dsa") ||
		strings.HasPrefix(lowered, "id_ecdsa") || strings.HasPrefix(lowered, "id_ed25519") {
		return true
	}
	if strings.Contains(lowered, "credential") || strings.Contains(lowered, "secret") ||
		strings.Contains(lowered, "password") || strings.HasPrefix(lowered, "private-key") {
		return true
	}
	return false
}

// RelativePath validates a slash-separated repository path.
func RelativePath(value string, allowEmpty bool) (string, error) {
	if value == "" && allowEmpty {
		return "", nil
	}
	if value == "" || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) ||
		strings.Contains(value, `\`) || strings.ContainsRune(value, 0) {
		return "", workerError("invalid_path", 400)
	}
	if drivePattern.MatchString(value) {
		return "", workerError("invalid_path", 400)
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", workerError("invalid_path", 400)
		}
	}
	for _, part := range parts {
		if SensitiveName(part) {
			return "", workerError("secret_path_forbidden", 403)
		}
	}
	lowered := make([]string, len(parts))
	for index, part := range parts {
		lowered[index] = strings.ToLower(part)
	}
	for index := 0; index+1 < len(lowered); index++ {
		if secretPaths[lowered[index]+"/"+lowered[index+1]] {
			return "", workerError("secret_path_forbidden", 403)
		}
	}
	return strings.Join(parts, "/"), nil
}

// SafePath resolves a validated repository path under root and rejects
// symlinks that could escape the workspace.
func SafePath(root, value string, allowEmpty, createParents bool) (string, error) {
	relative, err := RelativePath(value, allowEmpty)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", workerError("workspace_not_found", 404)
	}
	parts := []string{}
	if relative != "" {
		parts = strings.Split(relative, "/")
	}
	current := root
	limit := len(parts) - 1
	if limit < 0 {
		limit = 0
	}
	for _, part := range parts[:limit] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", workerError("symlink_forbidden", 403)
			}
			if !info.IsDir() {
				return "", workerError("invalid_path", 400)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", workerError("invalid_path", 400)
		}
		if !createParents {
			return "", workerError("path_not_found", 404)
		}
		if err := os.Mkdir(current, 0o750); err != nil {
			return "", workerError("invalid_path", 400)
		}
	}
	target := root
	for _, part := range parts {
		target = filepath.Join(target, part)
	}
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", workerError("symlink_forbidden", 403)
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", workerError("invalid_path", 400)
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return "", workerError("invalid_path", 400)
	}
	relativeToRoot, err := filepath.Rel(cleanRoot, cleanTarget)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return "", workerError("invalid_path", 400)
	}
	return target, nil
}

// TaskID converts a JSON value to a bounded workspace identifier.
func TaskID(value any) (int64, error) {
	switch typed := value.(type) {
	case bool:
		return 0, workerError("invalid_task", 400)
	case int:
		return boundedTaskID(int64(typed))
	case int64:
		return boundedTaskID(typed)
	case float64:
		if typed != float64(int64(typed)) {
			return 0, workerError("invalid_task", 400)
		}
		return boundedTaskID(int64(typed))
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		if err != nil {
			return 0, workerError("invalid_task", 400)
		}
		return boundedTaskID(parsed)
	case string:
		for _, r := range typed {
			if !unicode.IsDigit(r) {
				return 0, workerError("invalid_task", 400)
			}
		}
		parsed, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return 0, workerError("invalid_task", 400)
		}
		return boundedTaskID(parsed)
	default:
		return 0, workerError("invalid_task", 400)
	}
}

func boundedTaskID(value int64) (int64, error) {
	if value < 1 || value > TaskIDMax {
		return 0, workerError("invalid_task", 400)
	}
	return value, nil
}

// Slug validates a fixed identifier such as "client".
func Slug(value string) (string, error) {
	if !slugPattern.MatchString(value) {
		return "", workerError("invalid_slug", 400)
	}
	return value, nil
}

// BranchName validates a git branch name.
func BranchName(value string) (string, error) {
	if !branchPattern.MatchString(value) {
		return "", workerError("invalid_branch", 400)
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") ||
		strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") ||
		strings.HasSuffix(value, ".lock") {
		return "", workerError("invalid_branch", 400)
	}
	if strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.Contains(value, "//") {
		return "", workerError("invalid_branch", 400)
	}
	return value, nil
}

// RequireText validates a bounded text field.
func RequireText(value, name string, maximum int, allowEmpty bool) (string, error) {
	if !allowEmpty && strings.TrimSpace(value) == "" {
		return "", workerError("invalid_"+name, 400)
	}
	if len(value) > maximum || strings.ContainsRune(value, 0) {
		return "", workerError("invalid_"+name, 400)
	}
	return value, nil
}

// ImageReference validates a Docker image reference.
func ImageReference(value string) (string, error) {
	if !imagePattern.MatchString(value) {
		return "", fmt.Errorf("invalid image reference")
	}
	return value, nil
}
