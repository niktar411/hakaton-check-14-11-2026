package operations

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Listen, PublicURL, PlannerURL, AIURL, DatabaseURL, GitLabURL, OAuthID, OAuthSecret string
	GitLabToken, GitLabProject, GitLabWebhookSecret                                    string
	Provider                                                                           string
	AgentURL, AgentToken, WorkerURL, WorkerToken                                       string
	WorkerAgentURL, WorkerAgentToken                                                   string
	ModelURL, ModelKey, Model, AdminUser, AdminIP                                      string
	StaticDir, MihomoPath, AgentsPath                                                  string
	ModelEncryptionKey                                                                 []byte
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
		AIURL:       strings.TrimRight(os.Getenv("AI_BASE_URL"), "/"),
		DatabaseURL: os.Getenv("DATABASE_URL"), GitLabURL: strings.TrimRight(os.Getenv("GITLAB_URL"), "/"),
		Provider: envDefault("GIT_PROVIDER", "gitlab"),
		OAuthID:  os.Getenv("GITLAB_CLIENT_ID"), OAuthSecret: os.Getenv("GITLAB_CLIENT_SECRET"),
		GitLabToken: os.Getenv("GITLAB_TOKEN"), GitLabProject: os.Getenv("GITLAB_PROJECT"),
		GitLabWebhookSecret: os.Getenv("GITLAB_WEBHOOK_SECRET"),
		AgentURL:            strings.TrimRight(os.Getenv("AGENT_URL"), "/"), AgentToken: os.Getenv("AGENT_TOKEN"),
		WorkerURL: strings.TrimRight(os.Getenv("WORKER_URL"), "/"), WorkerToken: os.Getenv("WORKER_TOKEN"),
		WorkerAgentURL: strings.TrimRight(os.Getenv("WORKER_AGENT_URL"), "/"), WorkerAgentToken: os.Getenv("WORKER_AGENT_TOKEN"),
		ModelURL: strings.TrimRight(os.Getenv("MODEL_URL"), "/"), ModelKey: os.Getenv("MODEL_API_KEY"),
		Model: os.Getenv("MODEL_NAME"), AdminUser: envDefault("ADMIN_USERNAME", "niktar411"),
		AdminIP: envDefault("ADMIN_VPN_IP", "10.77.0.2"), StaticDir: os.Getenv("STATIC_DIR"),
		MihomoPath: envDefault("MIHOMO_CONFIG_PATH", "infra/mihomo/hackathon.yaml"),
		AgentsPath: envDefault("AGENTS_PATH", "/app/AGENTS.md"), ModelEncryptionKey: encryptionKey,
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
	urls := []string{c.PublicURL, c.PlannerURL, c.GitLabURL, c.AgentURL, c.WorkerURL}
	if c.AIURL != "" {
		urls = append(urls, c.AIURL)
	}
	if c.WorkerAgentURL != "" {
		urls = append(urls, c.WorkerAgentURL)
	}
	for _, address := range urls {
		u, parseErr := url.Parse(address)
		if parseErr != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return c, fmt.Errorf("invalid service URL")
		}
	}
	if err := validateModelEndpoint(c.ModelURL, c.Model); err != nil {
		return c, err
	}
	return c, nil
}

// The portal runs exactly one model: DeepSeek V4.1 Flash. Cloud.ru and
// OpenCode Go use different model identifiers, so the endpoint and the name
// must agree and no other model can be selected through the environment.
func validateModelEndpoint(rawURL, model string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("model must use an allowed HTTPS endpoint")
	}
	switch u.Host {
	case "foundation-models.api.cloud.ru":
		if model != "deepseek-ai/DeepSeek-V4.1-Flash" {
			return fmt.Errorf("model must be DeepSeek V4.1 Flash")
		}
	case "opencode.ai":
		if model != "deepseek-v4.1-flash" {
			return fmt.Errorf("model must be DeepSeek V4.1 Flash")
		}
	default:
		return fmt.Errorf("model must use an allowed HTTPS endpoint")
	}
	return nil
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
