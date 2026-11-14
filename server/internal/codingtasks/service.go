package codingtasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
	"hackathon/internal/identity"
	"hackathon/internal/llm"
	"hackathon/internal/notifications"
)

var (
	ErrNotFound  = errors.New("coding task not found")
	ErrInvalid   = errors.New("invalid coding task input")
	ErrConflict  = errors.New("coding task state conflict")
	ErrForbidden = errors.New("coding task access denied")
	ErrBusy      = errors.New("coding task execution busy")
)

// Config contains only administrative authorization and worker connection
// settings. Model and GitLab credentials stay in their shared services.
type Config struct {
	AdminUser      string
	AdminIP        string
	WorkerURL      string
	WorkerToken    string
	AsyncExecution bool
}

// Model is implemented by llm.Service.
type Model interface {
	Complete(ctx context.Context, input llm.Request) (llm.Response, error)
}

// GitLab verifies that the worker really created the reported merge request.
type GitLab interface {
	MergeRequestByIID(ctx context.Context, iid int64) (gitlab.MergeRequest, error)
}

// Store is implemented by db.Queries. Keeping generated storage behind this
// interface also makes all authorization and tool-loop behavior unit-testable.
type Store interface {
	CreateCodingTask(ctx context.Context, arg db.CreateCodingTaskParams) (db.CodingTask, error)
	GetCodingTask(ctx context.Context, id int64) (db.CodingTask, error)
	GetOwnedCodingTask(ctx context.Context, arg db.GetOwnedCodingTaskParams) (db.CodingTask, error)
	ListOwnedCodingTasks(ctx context.Context, arg db.ListOwnedCodingTasksParams) ([]db.CodingTask, error)
	ListCodingTasks(ctx context.Context, arg db.ListCodingTasksParams) ([]db.CodingTask, error)
	SetCodingTaskClarification(ctx context.Context, arg db.SetCodingTaskClarificationParams) (db.CodingTask, error)
	StartCodingTask(ctx context.Context, arg db.StartCodingTaskParams) (db.CodingTask, error)
	SetCodingTaskMergeRequest(ctx context.Context, arg db.SetCodingTaskMergeRequestParams) (db.CodingTask, error)
	SetCodingTaskPhase(ctx context.Context, arg db.SetCodingTaskPhaseParams) (db.CodingTask, error)
	AddCodingMessage(ctx context.Context, arg db.AddCodingMessageParams) (db.CodingMessage, error)
	ListCodingMessages(ctx context.Context, arg db.ListCodingMessagesParams) ([]db.CodingMessage, error)
	GetLocalAccountByID(ctx context.Context, id int64) (db.LocalAccount, error)
}

type taskLock struct {
	mu   sync.Mutex
	refs int
}

// Service owns task transitions. In particular, no Worker method is reachable
// before StartCodingTask has atomically consumed awaiting_confirmation.
type Service struct {
	store  Store
	model  Model
	worker Worker
	gitLab GitLab
	config Config

	adminPlanner     AdminPlanner
	adminDeployments AdminDeployments
	notifier         notifications.Notifier
	locksMu          sync.Mutex
	locks            map[int64]*taskLock
	execSlots        chan struct{}
}

func NewService(store Store, model Model, worker Worker, gitLab GitLab, config Config) *Service {
	return &Service{store: store, model: model, worker: worker, gitLab: gitLab, config: config, locks: make(map[int64]*taskLock), execSlots: make(chan struct{}, 3)}
}

// Recover fails tasks that were left running by a previous process. A goroutine
// cannot survive a restart, so keeping the phase running would make clients
// poll forever and block a new execution.
func (s *Service) Recover(ctx context.Context) error {
	rows, err := s.store.ListCodingTasks(ctx, db.ListCodingTasksParams{Limit: 1000, Offset: 0})
	if err != nil {
		return fmt.Errorf("list running coding tasks: %w", err)
	}
	for _, task := range rows {
		if task.Phase != PhaseRunning {
			continue
		}
		if _, failErr := s.fail(ctx, task.ID, "worker_restarted"); failErr != nil {
			return failErr
		}
	}
	return nil
}

func (s *Service) lockTask(id int64) func() {
	s.locksMu.Lock()
	lock := s.locks[id]
	if lock == nil {
		lock = &taskLock{}
		s.locks[id] = lock
	}
	lock.refs++
	s.locksMu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.locksMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.locks, id)
		}
		s.locksMu.Unlock()
	}
}

func (s *Service) account(ctx context.Context) (identity.Principal, db.LocalAccount, error) {
	principal, err := identity.FromContext(ctx)
	if err != nil {
		return identity.Principal{}, db.LocalAccount{}, err
	}
	if !principal.Session.AccountID.Valid || principal.Session.AccountID.Int64 < 1 {
		return identity.Principal{}, db.LocalAccount{}, ErrForbidden
	}
	account, err := s.store.GetLocalAccountByID(ctx, principal.Session.AccountID.Int64)
	if err != nil {
		return identity.Principal{}, db.LocalAccount{}, fmt.Errorf("load coding task account: %w", err)
	}
	if !account.Enabled || account.ID != principal.Session.AccountID.Int64 || account.Username == "" || account.Username != principal.Session.Username {
		return identity.Principal{}, db.LocalAccount{}, ErrForbidden
	}
	return principal, account, nil
}

func (s *Service) owned(ctx context.Context, id, accountID int64) (db.CodingTask, error) {
	task, err := s.store.GetOwnedCodingTask(ctx, db.GetOwnedCodingTaskParams{ID: id, AccountID: accountID})
	if isNotFound(err) {
		return db.CodingTask{}, ErrNotFound
	}
	if err != nil {
		return db.CodingTask{}, fmt.Errorf("load owned coding task: %w", err)
	}
	return task, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows)
}

func validText(value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max || strings.ContainsRune(value, '\x00') {
		return "", ErrInvalid
	}
	return value, nil
}

// Create always inserts a clarifying task and obtains a fresh clarification
// from the model. A parent does not inherit an executable phase or branch.
func (s *Service) Create(ctx context.Context, request string, parentID *int64) (CodingTask, error) {
	_, account, err := s.account(ctx)
	if err != nil {
		return CodingTask{}, err
	}
	request, err = validText(request, 16000)
	if err != nil {
		return CodingTask{}, err
	}
	var parent pgtype.Int8
	if parentID != nil {
		if *parentID < 1 {
			return CodingTask{}, ErrInvalid
		}
		if _, err := s.owned(ctx, *parentID, account.ID); err != nil {
			return CodingTask{}, err
		}
		parent = pgtype.Int8{Int64: *parentID, Valid: true}
	}
	return s.createClarifying(ctx, account, parent, request)
}

func (s *Service) createClarifying(ctx context.Context, account db.LocalAccount, parent pgtype.Int8, request string) (CodingTask, error) {
	task, err := s.store.CreateCodingTask(ctx, db.CreateCodingTaskParams{AccountID: account.ID, ParentID: parent, RequestText: request})
	if err != nil {
		return CodingTask{}, fmt.Errorf("create coding task: %w", err)
	}
	if task.Phase != PhaseClarifying {
		return CodingTask{}, fmt.Errorf("create coding task: invalid initial phase")
	}
	if _, err := s.store.AddCodingMessage(ctx, db.AddCodingMessageParams{TaskID: task.ID, Role: "user", Content: request}); err != nil {
		return CodingTask{}, fmt.Errorf("store coding task request: %w", err)
	}
	task, err = s.clarify(ctx, task, account)
	if err != nil {
		return CodingTask{}, err
	}
	return taskDTO(task), nil
}

// Answer discards the previous authorization to execute and requires another
// explicit confirmation after a new model clarification.
func (s *Service) Answer(ctx context.Context, id int64, answer string) (CodingTask, error) {
	_, account, err := s.account(ctx)
	if err != nil {
		return CodingTask{}, err
	}
	answer, err = validText(answer, 16000)
	if err != nil {
		return CodingTask{}, err
	}
	unlock := s.lockTask(id)
	defer unlock()
	task, err := s.owned(ctx, id, account.ID)
	if err != nil {
		return CodingTask{}, err
	}
	if task.Phase != PhaseAwaitingConfirmation {
		return CodingTask{}, ErrConflict
	}
	task, err = s.store.SetCodingTaskPhase(ctx, db.SetCodingTaskPhaseParams{ID: id, Phase: PhaseClarifying})
	if err != nil {
		return CodingTask{}, fmt.Errorf("re-enter coding task clarification: %w", err)
	}
	if _, err := s.store.AddCodingMessage(ctx, db.AddCodingMessageParams{TaskID: id, Role: "user", Content: answer}); err != nil {
		return CodingTask{}, fmt.Errorf("store coding task answer: %w", err)
	}
	task, err = s.clarify(ctx, task, account)
	if err != nil {
		return CodingTask{}, err
	}
	return taskDTO(task), nil
}

func (s *Service) clarify(ctx context.Context, task db.CodingTask, account db.LocalAccount) (db.CodingTask, error) {
	if s.model == nil {
		return s.fail(ctx, task.ID, "model_unavailable")
	}
	history, err := s.store.ListCodingMessages(ctx, db.ListCodingMessagesParams{TaskID: task.ID, Limit: 200})
	if err != nil {
		return db.CodingTask{}, fmt.Errorf("load clarification history: %w", err)
	}
	messages := []llm.Message{{Role: "system", Content: clarificationPrompt(account)}}
	for _, row := range history {
		if row.Role == "user" || row.Role == "assistant" {
			messages = append(messages, llm.Message{Role: row.Role, Content: row.Content})
		}
	}
	response, err := s.model.Complete(ctx, llm.Request{Messages: messages, MaxTokens: 1200, ResponseFormat: &llm.ResponseFormat{Type: "json_object"}, Actor: account.Username, RequestID: fmt.Sprintf("coding-clarify-%d-%d", task.ID, len(history)), Session: fmt.Sprintf("coding-%d", task.ID)})
	if err != nil {
		return s.fail(ctx, task.ID, "model_clarification_failed")
	}
	questions, err := parseClarification(response.Message.Content)
	if err != nil || len(response.Message.ToolCalls) != 0 {
		return s.fail(ctx, task.ID, "model_invalid_clarification")
	}
	clarification := strings.Join(questions, "\n")
	if _, err := s.store.AddCodingMessage(ctx, db.AddCodingMessageParams{TaskID: task.ID, Role: "assistant", Content: clarification}); err != nil {
		return db.CodingTask{}, fmt.Errorf("store coding task clarification: %w", err)
	}
	task, err = s.store.SetCodingTaskClarification(ctx, db.SetCodingTaskClarificationParams{ID: task.ID, Clarification: clarification})
	if isNotFound(err) {
		return db.CodingTask{}, ErrConflict
	}
	if err != nil {
		return db.CodingTask{}, fmt.Errorf("store coding task clarification phase: %w", err)
	}
	return task, nil
}

func parseClarification(content string) ([]string, error) {
	content = strings.TrimSpace(content)
	if content == "" || len(content) > 16000 {
		return nil, ErrInvalid
	}
	var decoded struct {
		Questions []string `json:"questions"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err == nil && decoded.Questions != nil {
		return cleanQuestions(decoded.Questions)
	}
	lines := strings.Split(content, "\n")
	return cleanQuestions(lines)
}

func cleanQuestions(lines []string) ([]string, error) {
	if len(lines) < 1 || len(lines) > 3 {
		return nil, ErrInvalid
	}
	questions := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || len(line) > 4000 || strings.ContainsRune(line, '\x00') {
			return nil, ErrInvalid
		}
		questions = append(questions, line)
	}
	return questions, nil
}

// Confirm consumes the current clarification phase before calling any worker
// operation, including read-only ones.
func (s *Service) Confirm(ctx context.Context, id int64, confirmation string) (CodingTask, error) {
	_, account, err := s.account(ctx)
	if err != nil {
		return CodingTask{}, err
	}
	confirmation, err = validText(confirmation, 200)
	if err != nil || !explicitConfirmation(confirmation) {
		return CodingTask{}, ErrInvalid
	}
	if s.config.AsyncExecution {
		select {
		case s.execSlots <- struct{}{}:
		default:
			return CodingTask{}, ErrBusy
		}
	}
	launched := false
	defer func() {
		if s.config.AsyncExecution && !launched {
			<-s.execSlots
		}
	}()
	unlock := s.lockTask(id)
	defer unlock()
	task, err := s.startConfirmed(ctx, id, account, confirmation)
	if err != nil {
		return CodingTask{}, err
	}
	if !s.config.AsyncExecution {
		task, err = s.execute(ctx, task, account)
		if err != nil {
			return CodingTask{}, err
		}
		return taskDTO(task), nil
	}
	jobCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() { <-s.execSlots }()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(jobCtx, "coding task execution panicked", "task_id", id, "panic", recovered)
				if _, failErr := s.fail(jobCtx, id, "worker_failed"); failErr != nil {
					slog.ErrorContext(jobCtx, "mark panicked coding task failed", "task_id", id, "error", failErr)
				}
			}
		}()
		unlock := s.lockTask(id)
		defer unlock()
		if _, execErr := s.execute(jobCtx, task, account); execErr != nil {
			slog.ErrorContext(jobCtx, "coding task execution failed", "task_id", id, "error", execErr)
			current, getErr := s.store.GetCodingTask(jobCtx, id)
			if getErr == nil && current.Phase == PhaseRunning {
				if _, failErr := s.fail(jobCtx, id, "worker_failed"); failErr != nil {
					slog.ErrorContext(jobCtx, "mark failed coding task", "task_id", id, "error", failErr)
				}
			}
		}
	}()
	launched = true
	return taskDTO(task), nil
}

// startConfirmed atomically consumes the confirmation and creates the branch.
// The caller owns the task lock.
func (s *Service) startConfirmed(ctx context.Context, id int64, account db.LocalAccount, confirmation string) (db.CodingTask, error) {
	task, err := s.owned(ctx, id, account.ID)
	if err != nil {
		return db.CodingTask{}, err
	}
	if task.Phase != PhaseAwaitingConfirmation {
		return db.CodingTask{}, ErrConflict
	}
	branch := fmt.Sprintf("coding/task-%d", id)
	task, err = s.store.StartCodingTask(ctx, db.StartCodingTaskParams{ID: id, AccountID: account.ID, Branch: branch})
	if isNotFound(err) {
		return db.CodingTask{}, ErrConflict
	}
	if err != nil {
		return db.CodingTask{}, fmt.Errorf("start coding task: %w", err)
	}
	if _, err := s.store.AddCodingMessage(ctx, db.AddCodingMessageParams{TaskID: id, Role: "user", Content: confirmation}); err != nil {
		return db.CodingTask{}, fmt.Errorf("store coding task confirmation: %w", err)
	}
	return task, nil
}

func explicitConfirmation(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Trim(value, " \t\r\n.!")
	switch value {
	case "confirm", "confirmed", "i confirm", "understood", "i understand", "yes i understand", "yes, i understand",
		"подтверждаю", "я подтверждаю", "понятно", "понял", "поняла", "да подтверждаю", "да, подтверждаю", "да, понятно":
		return true
	default:
		return false
	}
}

// Rework always creates a distinct task. The original task, including a
// confirmed or completed phase, is never mutated.
func (s *Service) Rework(ctx context.Context, id int64, request string) (CodingTask, error) {
	_, account, err := s.account(ctx)
	if err != nil {
		return CodingTask{}, err
	}
	request, err = validText(request, 16000)
	if err != nil {
		return CodingTask{}, err
	}
	parent, err := s.owned(ctx, id, account.ID)
	if err != nil {
		return CodingTask{}, err
	}
	return s.createClarifying(ctx, account, pgtype.Int8{Int64: parent.ID, Valid: true}, request)
}

func (s *Service) Get(ctx context.Context, id int64) (CodingTaskDetails, error) {
	_, account, err := s.account(ctx)
	if err != nil {
		return CodingTaskDetails{}, err
	}
	task, err := s.owned(ctx, id, account.ID)
	if err != nil {
		return CodingTaskDetails{}, err
	}
	return s.details(ctx, task)
}

func (s *Service) List(ctx context.Context, offset int32) ([]CodingTask, error) {
	_, account, err := s.account(ctx)
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, ErrInvalid
	}
	rows, err := s.store.ListOwnedCodingTasks(ctx, db.ListOwnedCodingTasksParams{AccountID: account.ID, Limit: 100, Offset: offset})
	if err != nil {
		return nil, fmt.Errorf("list owned coding tasks: %w", err)
	}
	return taskListDTO(rows), nil
}

func (s *Service) AdminGet(ctx context.Context, id int64) (CodingTaskDetails, error) {
	if _, err := identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP); err != nil {
		return CodingTaskDetails{}, err
	}
	task, err := s.store.GetCodingTask(ctx, id)
	if isNotFound(err) {
		return CodingTaskDetails{}, ErrNotFound
	}
	if err != nil {
		return CodingTaskDetails{}, fmt.Errorf("load coding task for admin: %w", err)
	}
	return s.details(ctx, task)
}

func (s *Service) AdminList(ctx context.Context, offset int32) ([]CodingTask, error) {
	if _, err := identity.RequireAdmin(ctx, s.config.AdminUser, s.config.AdminIP); err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, ErrInvalid
	}
	rows, err := s.store.ListCodingTasks(ctx, db.ListCodingTasksParams{Limit: 100, Offset: offset})
	if err != nil {
		return nil, fmt.Errorf("list coding tasks for admin: %w", err)
	}
	return taskListDTO(rows), nil
}

func (s *Service) details(ctx context.Context, task db.CodingTask) (CodingTaskDetails, error) {
	rows, err := s.store.ListCodingMessages(ctx, db.ListCodingMessagesParams{TaskID: task.ID, Limit: 1000})
	if err != nil {
		return CodingTaskDetails{}, fmt.Errorf("list coding task messages: %w", err)
	}
	messages := make([]CodingMessage, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, CodingMessage{ID: row.ID, Role: row.Role, Content: row.Content, ToolName: row.ToolName, CreatedAt: row.CreatedAt.Time})
	}
	return CodingTaskDetails{CodingTask: taskDTO(task), Messages: messages}, nil
}

func (s *Service) fail(ctx context.Context, id int64, reason string) (db.CodingTask, error) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	task, err := s.store.SetCodingTaskPhase(persistCtx, db.SetCodingTaskPhaseParams{ID: id, Phase: PhaseFailed, FailureReason: reason})
	if err != nil {
		return db.CodingTask{}, fmt.Errorf("mark coding task failed: %w", err)
	}
	return task, nil
}

func taskDTO(task db.CodingTask) CodingTask {
	return CodingTask{
		ID: task.ID, AccountID: task.AccountID, ParentID: nullableInt(task.ParentID), Request: task.RequestText,
		Phase: task.Phase, Clarification: task.Clarification, Branch: task.Branch,
		MergeRequestIID: nullableInt(task.MergeRequestIid), MergeRequestURL: task.MergeRequestUrl,
		FailureReason: task.FailureReason, CreatedAt: task.CreatedAt.Time, UpdatedAt: task.UpdatedAt.Time,
	}
}

func taskListDTO(rows []db.CodingTask) []CodingTask {
	result := make([]CodingTask, 0, len(rows))
	for _, row := range rows {
		result = append(result, taskDTO(row))
	}
	return result
}

func nullableInt(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
