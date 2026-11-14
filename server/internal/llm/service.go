package llm

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "hackathon/internal/db/sqlc"
)

const modelKeySetting = "deepseek_api_key"

type Config struct {
	URL           string
	APIKey        string
	Model         string
	EncryptionKey []byte
}

type Store interface {
	GetApplicationSetting(ctx context.Context, key string) (db.ApplicationSetting, error)
	UpsertApplicationSetting(ctx context.Context, arg db.UpsertApplicationSettingParams) (db.ApplicationSetting, error)
	BeginUsage(ctx context.Context, arg db.BeginUsageParams) (int64, error)
	FinishUsage(ctx context.Context, arg db.FinishUsageParams) error
}

type Service struct {
	db     Store
	config Config
	http   *http.Client
}

func NewService(pool *pgxpool.Pool, config Config, client *http.Client) *Service {
	return newService(db.New(pool), config, client)
}

func newService(store Store, config Config, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &Service{db: store, config: config, http: client}
}

func (s *Service) SetAPIKey(ctx context.Context, value string) error {
	value = strings.TrimSpace(value)
	if len(value) < 20 || len(value) > 4096 || len(s.config.EncryptionKey) != 32 {
		return fmt.Errorf("invalid model key or encryption configuration")
	}
	block, err := aes.NewCipher(s.config.EncryptionKey)
	if err != nil {
		return fmt.Errorf("create settings cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create settings GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("create settings nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	_, err = s.db.UpsertApplicationSetting(ctx, db.UpsertApplicationSettingParams{Key: modelKeySetting, Value: base64.RawStdEncoding.EncodeToString(sealed)})
	if err != nil {
		return fmt.Errorf("store model key: %w", err)
	}
	return nil
}

func (s *Service) credentials(ctx context.Context) (string, string, string, error) {
	key := s.config.APIKey
	setting, err := s.db.GetApplicationSetting(ctx, modelKeySetting)
	if err == nil {
		key, err = s.decrypt(setting.Value)
		if err != nil {
			return "", "", "", err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", fmt.Errorf("load model key: %w", err)
	}
	if key == "" || s.config.URL == "" || s.config.Model == "" {
		return "", "", "", fmt.Errorf("model is not configured")
	}
	return s.config.URL, key, s.config.Model, nil
}

func usageToken(value *int64) pgtype.Int8 {
	if value == nil || *value < 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func (s *Service) decrypt(value string) (string, error) {
	data, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode model key: %w", err)
	}
	block, err := aes.NewCipher(s.config.EncryptionKey)
	if err != nil {
		return "", fmt.Errorf("create settings cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create settings GCM: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted model key is truncated")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt model key: %w", err)
	}
	return string(plain), nil
}

func (s *Service) Ready(ctx context.Context) bool {
	_, _, _, err := s.credentials(ctx)
	return err == nil
}

func (s *Service) Complete(ctx context.Context, input Request) (Response, error) {
	baseURL, key, model, err := s.credentials(ctx)
	if err != nil {
		return Response{}, err
	}
	usageID := int64(0)
	usageStatus := "failed"
	usage := (*Usage)(nil)
	if input.RequestID != "" {
		if input.Actor == "" || len(input.RequestID) > 120 {
			return Response{}, fmt.Errorf("invalid model usage identity")
		}
		usageID, err = s.db.BeginUsage(ctx, db.BeginUsageParams{RequestID: input.RequestID, Username: input.Actor, Model: model})
		if err != nil {
			return Response{}, fmt.Errorf("begin model usage: %w", err)
		}
		defer func() {
			finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			finish := db.FinishUsageParams{ID: usageID, Status: usageStatus}
			if usage != nil {
				finish.InputTokens = usageToken(usage.PromptTokens)
				finish.OutputTokens = usageToken(usage.CompletionTokens)
				finish.TotalTokens = usageToken(usage.TotalTokens)
			}
			if err := s.db.FinishUsage(finishCtx, finish); err != nil {
				slog.ErrorContext(finishCtx, "finish model usage failed", "usage_id", usageID)
			}
		}()
	}
	payload := struct {
		Model string `json:"model"`
		Request
		Stream bool `json:"stream"`
	}{Model: model, Request: input, Stream: false}
	data, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("encode model request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return Response{}, fmt.Errorf("create model request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.http.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("call model: %w", err)
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			slog.WarnContext(ctx, "close model response failed", "error", closeErr)
		}
	}()
	if res.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("model returned status %d", res.StatusCode)
	}
	var decoded struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&decoded); err != nil {
		return Response{}, fmt.Errorf("decode model response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("model returned no choices")
	}
	usage = decoded.Usage
	usageStatus = "usage_missing"
	if usage != nil && usageToken(usage.PromptTokens).Valid && usageToken(usage.CompletionTokens).Valid && usageToken(usage.TotalTokens).Valid {
		usageStatus = "completed"
	}
	return Response{Message: decoded.Choices[0].Message, Usage: decoded.Usage}, nil
}
