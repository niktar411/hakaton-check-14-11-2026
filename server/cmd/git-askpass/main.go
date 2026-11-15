package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	prompt := ""
	if len(os.Args) > 1 {
		prompt = strings.ToLower(os.Args[1])
	}
	if strings.Contains(prompt, "username") {
		_, _ = fmt.Fprintln(os.Stdout, envOr("GITLAB_USERNAME", "oauth2"))
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, os.Getenv("GITLAB_TOKEN"))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
