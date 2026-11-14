package operations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
)

const chatGeneralContextKey = "chat_general_context"
const chatGeneralContextLimit = 16000
const agentsContextLimit = 16000

const skipClarificationPrompt = "продолжи работу на свое усмотрение"

const accessInstruction = "У пользователя и команды есть доступ к серверу, репозиторию и деплою. Ты не выполняешь команды и не меняешь файлы из этого чата; предлагай точные команды, файлы и шаги, которые пользователь может выполнить."

// ChatContextView is the administrator-facing snapshot of the general context
// that every chat receives. Personal per-user context is added separately.
type ChatContextView struct {
	General          string `json:"general"`
	Agents           string `json:"agents"`
	ClarifyingPrompt string `json:"clarifyingPrompt"`
	SolutionPrompt   string `json:"solutionPrompt"`
	SkipPhrase       string `json:"skipPhrase"`
	Limit            int    `json:"limit"`
}

type chatContextOutput struct{ Body ChatContextView }

type chatContextUpdateInput struct {
	Body struct {
		General string `json:"general" maxLength:"16000"`
	}
}

func (s *Service) generalChatContext(ctx context.Context) (string, error) {
	setting, err := s.db.GetApplicationSetting(ctx, chatGeneralContextKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load general chat context: %w", err)
	}
	return clipRunes(setting.Value, chatGeneralContextLimit), nil
}

// agentsContext reads the project instructions from the server. The file is
// mounted into the API container read-only, so an update on the host is picked
// up by the next chat request without changing the chat code.
func (s *Service) agentsContext() string {
	paths := []string{s.config.AgentsPath, "AGENTS.md", "../AGENTS.md"}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return clipRunes(string(data), agentsContextLimit)
	}
	slog.Warn("AGENTS.md is not available for the chat context")
	return ""
}

const projectSnapshotLimit = 24000

func composeChatInstruction(phase, agents, project, general, personal string) string {
	base := clarifyInstruction
	if phase == "ready" {
		base = solutionInstruction
	}
	parts := []string{base, accessInstruction}
	if strings.TrimSpace(agents) != "" {
		parts = append(parts, "Информация о проекте из AGENTS.md (файл на сервере):\n"+strings.TrimSpace(agents))
	}
	if strings.TrimSpace(project) != "" {
		parts = append(parts, "Снимок проекта на сервере:\n"+strings.TrimSpace(project))
	}
	if strings.TrimSpace(general) != "" {
		parts = append(parts, "Дополнительный общий контекст команды:\n"+strings.TrimSpace(general))
	}
	if strings.TrimSpace(personal) != "" {
		parts = append(parts, strings.TrimSpace(personal))
	}
	return strings.Join(parts, "\n\n")
}

// projectContext returns a cached plan and repository snapshot. The cache keeps
// chat latency and Gitea API traffic bounded; the plan still comes from the
// database without GitLab progress calls.
func (s *Service) projectContext(ctx context.Context) string {
	s.projectMu.Lock()
	if time.Since(s.projectAt) < time.Minute && s.projectText != "" {
		text := s.projectText
		s.projectMu.Unlock()
		return text
	}
	s.projectMu.Unlock()
	text := s.buildProjectContext(ctx)
	s.projectMu.Lock()
	s.projectAt = time.Now()
	s.projectText = text
	s.projectMu.Unlock()
	return text
}

func (s *Service) buildProjectContext(ctx context.Context) string {
	parts := make([]string, 0, 4)
	if s.planner != nil {
		blocks, err := s.planner.Snapshot(ctx)
		if err != nil {
			slog.WarnContext(ctx, "load planner snapshot for chat failed", "error", err)
		} else if len(blocks) > 0 {
			var builder strings.Builder
			builder.WriteString("План команды:")
			for _, block := range blocks {
				builder.WriteString(fmt.Sprintf("\n- #%d [%s] %s — %s", block.ID, block.Progress.State, block.Title, block.Owner.DisplayName))
				if block.Branch != "" {
					builder.WriteString(" · ветка " + block.Branch)
				}
				if block.MRIID != nil {
					builder.WriteString(fmt.Sprintf(" · MR !%d", *block.MRIID))
				}
			}
			parts = append(parts, builder.String())
		}
	}
	if s.gitLab != nil {
		branch, err := s.gitLab.DefaultBranch(ctx)
		if err != nil {
			slog.WarnContext(ctx, "load repository branch for chat failed", "error", err)
		} else {
			if entries, treeErr := s.gitLab.Tree(ctx, branch); treeErr != nil {
				slog.WarnContext(ctx, "load repository tree for chat failed", "error", treeErr)
			} else {
				parts = append(parts, formatRepositoryTree(branch, entries))
			}
			parts = append(parts, s.readKeyFiles(ctx, branch)...)
		}
	}
	return clipRunes(strings.Join(parts, "\n\n"), projectSnapshotLimit)
}

func formatRepositoryTree(branch string, entries []gitlab.TreeEntry) string {
	var builder strings.Builder
	builder.WriteString("Файлы репозитория (ветка " + branch + ", первые 220):")
	shown := 0
	for _, entry := range entries {
		if shown >= 220 {
			break
		}
		if entry.Type != "blob" {
			continue
		}
		if hiddenRepositoryPath(entry.Path) {
			continue
		}
		builder.WriteString("\n- " + entry.Path)
		shown++
	}
	if shown == 0 {
		return ""
	}
	return builder.String()
}

func hiddenRepositoryPath(path string) bool {
	lower := strings.ToLower(path)
	for _, prefix := range []string{"node_modules/", "dist/", ".git/", "test-results/", "coverage/", "playwright-report/", "client/src/paraglide/", "client/project.inlang/cache/"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func (s *Service) readKeyFiles(ctx context.Context, branch string) []string {
	paths := []string{"CLAUDE.md", "README.md", "package.json", "server/go.mod"}
	parts := make([]string, 0, len(paths))
	for _, path := range paths {
		data, err := s.gitLab.File(ctx, path, branch)
		if err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("Файл %s:\n%s", path, clipRunes(string(data), 6000)))
	}
	return parts
}

// buildChatInstruction assembles the common context plus the user's personal
// style. The general context and AGENTS.md are shared by every participant.
func (s *Service) buildChatInstruction(ctx context.Context, phase string, account db.LocalAccount) string {
	general, err := s.generalChatContext(ctx)
	if err != nil {
		slog.WarnContext(ctx, "load general chat context failed", "error", err)
	}
	return composeChatInstruction(phase, s.agentsContext(), s.projectContext(ctx), general, personalInstruction(account))
}

func wantsSkipClarification(prompt string) bool {
	normalized := strings.ToLower(prompt)
	normalized = strings.ReplaceAll(normalized, "ё", "е")
	normalized = strings.Join(strings.Fields(normalized), " ")
	for _, phrase := range []string{
		"продолжи работу на свое усмотрение",
		"продолжай работу на свое усмотрение",
		"продолжи на свое усмотрение",
		"действуй на свое усмотрение",
	} {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

// clarificationAlreadyAsked reports whether the AI has already produced one
// clarifying answer. The next user message then goes straight to a solution.
func clarificationAlreadyAsked(history []db.Message) bool {
	for _, message := range history {
		if message.Role == "assistant" {
			return true
		}
	}
	return false
}

func (s *Service) ChatContext(ctx context.Context) (*chatContextOutput, error) {
	if _, err := s.admin(ctx); err != nil {
		return nil, err
	}
	general, err := s.generalChatContext(ctx)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	agents := s.agentsContext()
	project := s.projectContext(ctx)
	return &chatContextOutput{Body: ChatContextView{
		General:          general,
		Agents:           agents,
		ClarifyingPrompt: composeChatInstruction("clarifying", agents, project, general, ""),
		SolutionPrompt:   composeChatInstruction("ready", agents, project, general, ""),
		SkipPhrase:       skipClarificationPrompt,
		Limit:            chatGeneralContextLimit,
	}}, nil
}

func (s *Service) UpdateChatContext(ctx context.Context, input *chatContextUpdateInput) (*chatContextOutput, error) {
	principal, err := s.admin(ctx)
	if err != nil {
		return nil, err
	}
	general := strings.TrimSpace(input.Body.General)
	if utf8.RuneCountInString(general) > chatGeneralContextLimit {
		return nil, huma.Error400BadRequest("chat.context_too_long")
	}
	auditID, err := s.db.BeginAudit(ctx, db.BeginAuditParams{Username: principal.Session.Username, Action: "update_chat_context", Target: "chat"})
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	status := "completed"
	_, saveErr := s.db.UpsertApplicationSetting(ctx, db.UpsertApplicationSettingParams{Key: chatGeneralContextKey, Value: general})
	if saveErr != nil {
		status = "failed"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.db.FinishAudit(finishCtx, db.FinishAuditParams{ID: auditID, Status: status}); err != nil {
		return nil, huma.Error503ServiceUnavailable("audit.unknown_outcome")
	}
	if saveErr != nil {
		return nil, huma.Error503ServiceUnavailable("storage.unavailable")
	}
	return s.ChatContext(ctx)
}
