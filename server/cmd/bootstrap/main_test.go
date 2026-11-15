package main

import (
	"strings"
	"testing"
)

func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestLoadConfig(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":   "postgres://user:secret@localhost/portal",
		"ADMIN_USERNAME": "root",
		"ADMIN_PASSWORD": "bootstrap-password",
		"ADMIN_COLOR":    "#112233",
	}
	config, err := loadConfig(environment(values))
	if err != nil {
		t.Fatal(err)
	}
	if config.databaseURL != values["DATABASE_URL"] || config.username != "root" ||
		config.password != "bootstrap-password" || config.color != "#112233" {
		t.Fatalf("incorrect configuration: %#v", config)
	}
}

func TestLoadConfigRequiresEveryValueWithoutLeakingSecrets(t *testing.T) {
	for _, missing := range []string{"DATABASE_URL", "ADMIN_USERNAME", "ADMIN_PASSWORD", "ADMIN_COLOR"} {
		t.Run(missing, func(t *testing.T) {
			values := map[string]string{
				"DATABASE_URL":   "postgres://user:database-secret@localhost/portal",
				"ADMIN_USERNAME": "root",
				"ADMIN_PASSWORD": "password-secret",
				"ADMIN_COLOR":    "#112233",
			}
			delete(values, missing)
			_, err := loadConfig(environment(values))
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("missing %s returned %v", missing, err)
			}
			if strings.Contains(err.Error(), "database-secret") || strings.Contains(err.Error(), "password-secret") {
				t.Fatalf("configuration error leaked a secret: %v", err)
			}
		})
	}
}
