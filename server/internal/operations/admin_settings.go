package operations

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	db "hackathon/internal/db/sqlc"
)

type modelKeyInput struct {
	Body struct {
		Key string `json:"key" minLength:"20" maxLength:"4096"`
	}
}

type modelStatusOutput struct {
	Body struct {
		Configured bool `json:"configured"`
	}
}

type mihomoOutput struct {
	Body struct {
		Configuration string `json:"configuration"`
	}
}

func (s *Service) UpdateModelKey(ctx context.Context, input *modelKeyInput) (*modelStatusOutput, error) {
	principal, err := s.admin(ctx)
	if err != nil {
		return nil, err
	}
	auditID, err := s.db.BeginAudit(ctx, db.BeginAuditParams{Username: principal.Session.Username, Action: "update_model_key", Target: "deepseek"})
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	updateErr := s.model.SetAPIKey(ctx, input.Body.Key)
	status := "completed"
	if updateErr != nil {
		status = "failed"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.db.FinishAudit(finishCtx, db.FinishAuditParams{ID: auditID, Status: status}); err != nil {
		return nil, huma.Error503ServiceUnavailable("audit.unknown_outcome")
	}
	if updateErr != nil {
		return nil, huma.Error400BadRequest("model.key_rejected")
	}
	return &modelStatusOutput{Body: struct {
		Configured bool `json:"configured"`
	}{Configured: true}}, nil
}

func (s *Service) Mihomo(ctx context.Context) (*mihomoOutput, error) {
	if _, err := s.admin(ctx); err != nil {
		return nil, err
	}
	file, err := os.Open(s.config.MihomoPath)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("mihomo.unavailable")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.WarnContext(ctx, "close Mihomo configuration failed", "error", closeErr)
		}
	}()
	info, err := file.Stat()
	if err != nil || info.Size() > 1<<20 {
		return nil, huma.Error503ServiceUnavailable("mihomo.unavailable")
	}
	data := make([]byte, info.Size())
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, huma.Error503ServiceUnavailable("mihomo.unavailable")
	}
	configuration := string(data)
	lower := strings.ToLower(configuration)
	if !strings.Contains(configuration, "10.77.0.0/24") || strings.Contains(lower, "privatekey") || strings.Contains(lower, "private_key") || strings.Contains(lower, "private key") {
		return nil, huma.Error503ServiceUnavailable("mihomo.invalid_configuration")
	}
	return &mihomoOutput{Body: struct {
		Configuration string `json:"configuration"`
	}{Configuration: configuration}}, nil
}
