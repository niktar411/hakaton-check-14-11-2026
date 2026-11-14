package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"hackathon/internal/codingtasks"
	"hackathon/internal/deployments"
	"hackathon/internal/identity"
	"hackathon/internal/localaccounts"
	"hackathon/internal/plannerblocks"
	"hackathon/internal/questions"
	"hackathon/internal/sam/assistant"
)

func APIConfig() huma.Config {
	c := huma.DefaultConfig("Hackathon team operations", "1.0.0")
	c.DocsPath = ""
	c.OpenAPIPath = ""
	c.SchemasPath = ""
	return c
}

func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
			next.ServeHTTP(w, r)
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.WithRequestIP(r.Context(), remoteIP(r))))
		})
	})
	r.Get("/sitemap.xml", s.sitemap)
	r.Get("/robots.txt", s.robots)
	r.Get("/llms.txt", s.llms)
	r.Get("/admin/login", s.serveIndex)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.pool.Ping(r.Context()); err != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.WriteHeader(204)
	})
	r.Get("/auth/login", s.Login)
	r.Get("/auth/callback", s.Callback)
	r.Group(func(r chi.Router) {
		api := humachi.New(r, APIConfig())
		localaccounts.RegisterPublic(api, s.accounts)
		questions.RegisterPublic(api, s.questions)
		plannerblocks.RegisterPublic(api, s.planner)
	})
	r.Post("/api/webhooks/gitlab", s.deployments.HandleGitLabWebhook)
	r.Group(func(r chi.Router) {
		r.Use(s.Authenticate)
		r.Post("/api/chat/stream", s.ChatStream)
		api := humachi.New(r, APIConfig())
		Register(api, s)
		localaccounts.RegisterAdmin(api, s.accounts)
		questions.RegisterAdmin(api, s.questions)
		assistant.Register(api, s.samAssistant)
		plannerblocks.RegisterTeam(api, s.planner)
		plannerblocks.RegisterAdmin(api, s.planner)
		codingtasks.Register(api, s.coding)
		deployments.Register(api, s.deployments)
		deployments.RegisterMember(api, s.deployments)
		registerLogout(api, s)
		r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
			if _, err := s.admin(r.Context()); err != nil {
				http.Error(w, "auth.admin_vpn_required", 403)
				return
			}
			s.serveIndex(w, r)
		})
	})
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") || (strings.HasPrefix(r.URL.Path, "/admin/") && r.URL.Path != "/admin/login") {
			http.NotFound(w, r)
			return
		}
		rel := filepath.Clean("/" + r.URL.Path)
		file := filepath.Join(s.config.StaticDir, rel)
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			http.ServeFile(w, r, file)
			return
		}
		s.serveIndex(w, r)
	})
	return r
}

func registerLogout(api huma.API, s *Service) {
	huma.Register(api, huma.Operation{
		OperationID:   "logout",
		Method:        http.MethodPost,
		Path:          "/api/logout",
		Summary:       "Log out",
		Tags:          []string{"Authentication"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, _ *struct{}) (*logoutOutput, error) {
		if s == nil {
			return &logoutOutput{}, nil
		}
		return s.Logout(ctx)
	})
}

func WriteOpenAPI(ctx context.Context, path string) error {
	r := chi.NewRouter()
	api := humachi.New(r, APIConfig())
	Register(api, nil)
	localaccounts.RegisterPublic(api, nil)
	localaccounts.RegisterAdmin(api, nil)
	questions.RegisterPublic(api, nil)
	questions.RegisterAdmin(api, nil)
	assistant.Register(api, nil)
	plannerblocks.Register(api, nil)
	codingtasks.Register(api, nil)
	deployments.Register(api, nil)
	deployments.RegisterMember(api, nil)
	registerLogout(api, nil)
	data, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode API: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create API directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write API: %w", err)
	}
	return ctx.Err()
}
