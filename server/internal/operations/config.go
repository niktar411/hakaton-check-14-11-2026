package operations

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Listen, PublicURL, PlannerURL, DatabaseURL, GitLabURL, OAuthID, OAuthSecret string
	GitLabToken, GitLabProject, GitLabWebhookSecret                             string
	Provider                                                                    string
	AgentURL, AgentToken, WorkerURL, WorkerToken                                string
	ModelURL, ModelKey, Model, AdminUser, AdminIP                               string
	StaticDir, MihomoPath                                                       string
	ModelEncryptionKey                                                          []byte
}

func LoadConfig() (Config, error) {
	publicURL := os.Getenv("PUBLIC_BASE_URL")
	if publicURL == "" {
		publicURL = os.Getenv("PORTAL_URL")
	}
	encryptionKey, err := base64.RawStdEncoding.DecodeString(os.Getenv("SETTINGS_ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, fmt.Errorf("decode SETTINGS_ENCRYPTION_KEY: %w", err)
	}
	c := Config{
		Listen: os.Getenv("PORTAL_LISTEN"), PublicURL: strings.TrimRight(publicURL, "/"),
		PlannerURL:  strings.TrimRight(os.Getenv("PLANNER_BASE_URL"), "/"),
		DatabaseURL: os.Getenv("DATABASE_URL"), GitLabURL: strings.TrimRight(os.Getenv("GITLAB_URL"), "/"),
		Provider: envDefault("GIT_PROVIDER", "gitlab"),
		OAuthID: os.Getenv("GITLAB_CLIENT_ID"), OAuthSecret: os.Getenv("GITLAB_CLIENT_SECRET"),
		GitLabToken: os.Getenv("GITLAB_TOKEN"), GitLabProject: os.Getenv("GITLAB_PROJECT"),
		GitLabWebhookSecret: os.Getenv("GITLAB_WEBHOOK_SECRET"),
		AgentURL:            strings.TrimRight(os.Getenv("AGENT_URL"), "/"), AgentToken: os.Getenv("AGENT_TOKEN"),
		WorkerURL: strings.TrimRight(os.Getenv("WORKER_URL"), "/"), WorkerToken: os.Getenv("WORKER_TOKEN"),
		ModelURL: strings.TrimRight(os.Getenv("MODEL_URL"), "/"), ModelKey: os.Getenv("MODEL_API_KEY"),
		Model: os.Getenv("MODEL_NAME"), AdminUser: envDefault("ADMIN_USERNAME", "niktar411"),
		AdminIP: envDefault("ADMIN_VPN_IP", "10.77.0.2"), StaticDir: os.Getenv("STATIC_DIR"),
		MihomoPath: envDefault("MIHOMO_CONFIG_PATH", "infra/mihomo/hackathon.yaml"), ModelEncryptionKey: encryptionKey,
	}
	required := map[string]string{
		"PORTAL_LISTEN": c.Listen, "PUBLIC_BASE_URL": c.PublicURL, "PLANNER_BASE_URL": c.PlannerURL,
		"DATABASE_URL": c.DatabaseURL, "GITLAB_URL": c.GitLabURL, "GITLAB_CLIENT_ID": c.OAuthID,
		"GITLAB_CLIENT_SECRET": c.OAuthSecret, "GITLAB_TOKEN": c.GitLabToken, "GITLAB_PROJECT": c.GitLabProject,
		"GITLAB_WEBHOOK_SECRET": c.GitLabWebhookSecret, "AGENT_URL": c.AgentURL, "AGENT_TOKEN": c.AgentToken,
		"WORKER_URL": c.WorkerURL, "WORKER_TOKEN": c.WorkerToken, "STATIC_DIR": c.StaticDir,
		"MODEL_URL": c.ModelURL, "MODEL_NAME": c.Model,
	}
	for name, value := range required {
		if value == "" {
			return c, fmt.Errorf("missing configuration: %s", name)
		}
	}
	if len(c.ModelEncryptionKey) != 32 {
		return c, fmt.Errorf("SETTINGS_ENCRYPTION_KEY must decode to 32 bytes")
	}
	for _, address := range []string{c.PublicURL, c.PlannerURL, c.GitLabURL, c.AgentURL, c.WorkerURL} {
		u, parseErr := url.Parse(address)
		if parseErr != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return c, fmt.Errorf("invalid service URL")
		}
	}
	u, parseErr := url.Parse(c.ModelURL)
	if parseErr != nil || u.Scheme != "https" || u.Host != "foundation-models.api.cloud.ru" {
		return c, fmt.Errorf("model must use configured Cloud.ru HTTPS endpoint")
	}
	return c, nil
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
