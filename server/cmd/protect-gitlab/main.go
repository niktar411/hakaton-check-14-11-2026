package main

import (
	"fmt"
	"log/slog"
	"os"

	"hackathon/internal/infra/gitlabprotect"
)

func main() {
	client, err := gitlabprotect.New(gitlabprotect.Config{
		APIURL:    os.Getenv("GITLAB_API_URL"),
		Token:     os.Getenv("GITLAB_TOKEN"),
		Project:   os.Getenv("GITLAB_PROJECT"),
		AdminID:   os.Getenv("GITLAB_ADMIN_ID"),
		AdminName: os.Getenv("GITLAB_ADMIN_USERNAME"),
		Branch:    envOr("GITLAB_PROTECTED_BRANCH", "main"),
	})
	if err != nil {
		fail(err)
	}
	if err := client.Enforce(); err != nil {
		fail(err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "GitLab main protection verified: no direct push, successful pipeline, administrator approval")
}

func fail(err error) {
	slog.Error("GitLab protection failed", "error", err)
	os.Exit(1)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
