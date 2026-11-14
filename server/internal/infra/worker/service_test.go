package worker

import (
	"regexp"
	"testing"
)

func testConfig(t *testing.T, overrides func(*Config)) Config {
	t.Helper()
	directory := t.TempDir()
	config := Config{
		Workspaces: directory + "/workspaces", Reviews: directory + "/reviews", State: directory + "/state",
		CodingImage: "hackathon-worker-coding:local", PostgresImage: "postgres:18.1-alpine3.22",
		BaseBranch: "main", ReviewURL: "http://10.77.0.10:8080", ReviewBindIP: "10.77.0.10",
		GitAskPass:   "/usr/local/bin/hackathon-git-askpass",
		CheckTimeout: 105, MaxConcurrency: 2,
	}
	if overrides != nil {
		overrides(&config)
	}
	return config
}

func TestConfigValidation(t *testing.T) {
	if err := testConfig(t, nil).Validate(); err != nil {
		t.Fatalf("default config rejected: %v", err)
	}
	for _, reviewURL := range []string{"http://example.com:8080", "https://10.77.0.10:8080", "http://10.77.0.10:8080/team"} {
		config := testConfig(t, func(c *Config) { c.ReviewURL = reviewURL })
		if err := config.Validate(); err == nil {
			t.Fatalf("review URL %q accepted", reviewURL)
		}
	}
	for _, override := range []func(*Config){
		func(c *Config) { c.CheckTimeout = 5 },
		func(c *Config) { c.CheckTimeout = 111 },
		func(c *Config) { c.MaxConcurrency = 0 },
		func(c *Config) { c.MaxConcurrency = 5 },
		func(c *Config) { c.CodingImage = "bad image" },
		func(c *Config) { c.BaseBranch = "-main" },
		func(c *Config) { c.PostgresImage = "" },
	} {
		config := testConfig(t, override)
		if err := config.Validate(); err == nil {
			t.Fatalf("invalid config accepted: %+v", config)
		}
	}
}

func TestDeploymentAllowlist(t *testing.T) {
	worker, err := NewWorker(testConfig(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{
		{"restart", "production"}, {"deploy", "coding"}, {"start", "test"},
		{"create_test_database", "review"}, {"restart", "test"}, {"drop_test_database", "coding"},
	} {
		if _, err := worker.Deployment(pair[0], pair[1]); err == nil {
			t.Fatalf("deployment %v accepted", pair)
		}
	}
	if !regexp.MustCompile(`^\d+:\d+$`).MatchString(containerUser()) {
		t.Fatalf("container user = %q", containerUser())
	}
}

func TestPythonCheckIsNotAvailable(t *testing.T) {
	if Checks["python-test"] {
		t.Fatal("python-test must not be a worker check after the Go migration")
	}
}
