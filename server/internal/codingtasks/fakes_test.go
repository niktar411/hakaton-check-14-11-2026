package codingtasks

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
	"hackathon/internal/identity"
	"hackathon/internal/llm"
)

type fakeStore struct {
	mu       sync.Mutex
	nextTask int64
	nextMsg  int64
	tasks    map[int64]db.CodingTask
	messages map[int64][]db.CodingMessage
	accounts map[int64]db.LocalAccount
}

func newFakeStore() *fakeStore {
	now := pgtype.Timestamptz{Time: time.Unix(1700000000, 0).UTC(), Valid: true}
	return &fakeStore{
		nextTask: 1,
		nextMsg:  1,
		tasks:    make(map[int64]db.CodingTask),
		messages: make(map[int64][]db.CodingMessage),
		accounts: map[int64]db.LocalAccount{
			1: {ID: 1, Username: "alice", DisplayName: "Alice", Role: "member", Enabled: true, PersonalContext: "Maintains the parser", Tone: "Direct and concise", CreatedAt: now, UpdatedAt: now},
			2: {ID: 2, Username: "bob", DisplayName: "Bob", Role: "member", Enabled: true, CreatedAt: now, UpdatedAt: now},
			3: {ID: 3, Username: "root", DisplayName: "Root", Role: "admin", Enabled: true, CreatedAt: now, UpdatedAt: now},
		},
	}
}

func (f *fakeStore) CreateCodingTask(_ context.Context, arg db.CreateCodingTaskParams) (db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	task := db.CodingTask{ID: f.nextTask, AccountID: arg.AccountID, ParentID: arg.ParentID, RequestText: arg.RequestText, Phase: PhaseClarifying, CreatedAt: now, UpdatedAt: now}
	f.nextTask++
	f.tasks[task.ID] = task
	return task, nil
}

func (f *fakeStore) GetCodingTask(_ context.Context, id int64) (db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.tasks[id]
	if !ok {
		return db.CodingTask{}, pgx.ErrNoRows
	}
	return task, nil
}

func (f *fakeStore) GetOwnedCodingTask(_ context.Context, arg db.GetOwnedCodingTaskParams) (db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.tasks[arg.ID]
	if !ok || task.AccountID != arg.AccountID {
		return db.CodingTask{}, pgx.ErrNoRows
	}
	return task, nil
}

func (f *fakeStore) ListOwnedCodingTasks(_ context.Context, arg db.ListOwnedCodingTasksParams) ([]db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := make([]db.CodingTask, 0)
	for _, task := range f.tasks {
		if task.AccountID == arg.AccountID {
			rows = append(rows, task)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })
	return pageTasks(rows, arg.Limit, arg.Offset), nil
}

func (f *fakeStore) ListCodingTasks(_ context.Context, arg db.ListCodingTasksParams) ([]db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := make([]db.CodingTask, 0, len(f.tasks))
	for _, task := range f.tasks {
		rows = append(rows, task)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })
	return pageTasks(rows, arg.Limit, arg.Offset), nil
}

func pageTasks(rows []db.CodingTask, limit, offset int32) []db.CodingTask {
	if int(offset) >= len(rows) {
		return []db.CodingTask{}
	}
	rows = rows[offset:]
	if int(limit) < len(rows) {
		rows = rows[:limit]
	}
	return append([]db.CodingTask{}, rows...)
}

func (f *fakeStore) SetCodingTaskClarification(_ context.Context, arg db.SetCodingTaskClarificationParams) (db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.tasks[arg.ID]
	if !ok || task.Phase != PhaseClarifying {
		return db.CodingTask{}, pgx.ErrNoRows
	}
	task.Phase = PhaseAwaitingConfirmation
	task.Clarification = arg.Clarification
	task.UpdatedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	f.tasks[arg.ID] = task
	return task, nil
}

func (f *fakeStore) StartCodingTask(_ context.Context, arg db.StartCodingTaskParams) (db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.tasks[arg.ID]
	if !ok || task.AccountID != arg.AccountID || task.Phase != PhaseAwaitingConfirmation {
		return db.CodingTask{}, pgx.ErrNoRows
	}
	task.Phase = PhaseRunning
	task.Branch = arg.Branch
	task.FailureReason = ""
	f.tasks[arg.ID] = task
	return task, nil
}

func (f *fakeStore) SetCodingTaskMergeRequest(_ context.Context, arg db.SetCodingTaskMergeRequestParams) (db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.tasks[arg.ID]
	if !ok || task.Phase != PhaseRunning {
		return db.CodingTask{}, pgx.ErrNoRows
	}
	task.Phase = PhaseAwaitingMerge
	task.MergeRequestIid = arg.MergeRequestIid
	task.MergeRequestUrl = arg.MergeRequestUrl
	f.tasks[arg.ID] = task
	return task, nil
}

func (f *fakeStore) SetCodingTaskPhase(_ context.Context, arg db.SetCodingTaskPhaseParams) (db.CodingTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task, ok := f.tasks[arg.ID]
	if !ok {
		return db.CodingTask{}, pgx.ErrNoRows
	}
	task.Phase = arg.Phase
	task.FailureReason = arg.FailureReason
	f.tasks[arg.ID] = task
	return task, nil
}

func (f *fakeStore) AddCodingMessage(_ context.Context, arg db.AddCodingMessageParams) (db.CodingMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	message := db.CodingMessage{ID: f.nextMsg, TaskID: arg.TaskID, Role: arg.Role, Content: arg.Content, ToolName: arg.ToolName, CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}
	f.nextMsg++
	f.messages[arg.TaskID] = append(f.messages[arg.TaskID], message)
	return message, nil
}

func (f *fakeStore) ListCodingMessages(_ context.Context, arg db.ListCodingMessagesParams) ([]db.CodingMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := f.messages[arg.TaskID]
	if int(arg.Offset) >= len(rows) {
		return []db.CodingMessage{}, nil
	}
	rows = rows[arg.Offset:]
	if int(arg.Limit) < len(rows) {
		rows = rows[:arg.Limit]
	}
	return append([]db.CodingMessage{}, rows...), nil
}

func (f *fakeStore) GetLocalAccountByID(_ context.Context, id int64) (db.LocalAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	account, ok := f.accounts[id]
	if !ok {
		return db.LocalAccount{}, pgx.ErrNoRows
	}
	return account, nil
}

type fakeModel struct {
	mu        sync.Mutex
	responses []llm.Response
	errors    []error
	requests  []llm.Request
}

func (f *fakeModel) Complete(_ context.Context, input llm.Request) (llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	input.Messages = append([]llm.Message{}, input.Messages...)
	input.Tools = append([]llm.Tool{}, input.Tools...)
	f.requests = append(f.requests, input)
	if len(f.responses) == 0 {
		return llm.Response{}, errors.New("no scripted response")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	var err error
	if len(f.errors) > 0 {
		err = f.errors[0]
		f.errors = f.errors[1:]
	}
	return response, err
}

func (f *fakeModel) add(responses ...llm.Response) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, responses...)
}

func clarification(questions ...string) llm.Response {
	data, _ := json.Marshal(map[string][]string{"questions": questions})
	return llm.Response{Message: llm.Message{Role: "assistant", Content: string(data)}}
}

func toolResponse(calls ...llm.ToolCall) llm.Response {
	return llm.Response{Message: llm.Message{Role: "assistant", ToolCalls: calls}}
}

func call(id, name, arguments string) llm.ToolCall {
	return llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: arguments}}
}

func finalResponse(content string) llm.Response {
	return llm.Response{Message: llm.Message{Role: "assistant", Content: content}}
}

type workerCall struct {
	operation string
	input     ToolInput
}

type fakeWorker struct {
	mu    sync.Mutex
	calls []workerCall
	fn    func(operation string, input ToolInput) (ToolOutput, error)
}

func (f *fakeWorker) Execute(_ context.Context, operation string, input ToolInput) (ToolOutput, error) {
	f.mu.Lock()
	f.calls = append(f.calls, workerCall{operation: operation, input: input})
	fn := f.fn
	f.mu.Unlock()
	if fn != nil {
		return fn(operation, input)
	}
	switch operation {
	case ToolPrepareWorkspace, ToolWriteFile:
		return ToolOutput{OK: true}, nil
	case ToolRunCheck:
		return ToolOutput{OK: true, Passed: true, Output: "ok"}, nil
	case ToolCreateMergeRequest:
		return ToolOutput{OK: true, MergeRequestIID: 77, MergeRequestURL: "https://gitlab.example/team/project/-/merge_requests/77"}, nil
	default:
		return ToolOutput{OK: true}, nil
	}
}

func (f *fakeWorker) operations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		result = append(result, call.operation)
	}
	return result
}

type fakeGitLab struct {
	mu       sync.Mutex
	requests []int64
	byIID    map[int64]gitlab.MergeRequest
	err      error
}

func (f *fakeGitLab) MergeRequestByIID(_ context.Context, iid int64) (gitlab.MergeRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, iid)
	if f.err != nil {
		return gitlab.MergeRequest{}, f.err
	}
	if result, ok := f.byIID[iid]; ok {
		return result, nil
	}
	return gitlab.MergeRequest{}, errors.New("merge request absent")
}

func userContext(accountID int64, username string) context.Context {
	account := db.LocalAccount{ID: accountID, Username: username, Enabled: true, Role: "member"}
	return identity.WithPrincipal(context.Background(), identity.Principal{
		Session: db.Session{Username: username, AccountID: pgtype.Int8{Int64: accountID, Valid: true}},
		Account: account, AuthKind: "local", IP: "192.0.2.10",
	})
}

func adminContext() context.Context {
	account := db.LocalAccount{ID: 3, Username: "root", Enabled: true, Role: "admin"}
	return identity.WithPrincipal(context.Background(), identity.Principal{
		Session: db.Session{Username: "root", AccountID: pgtype.Int8{Int64: 3, Valid: true}},
		Account: account, AuthKind: "local", IP: "10.77.0.2",
	})
}

func mergeRequest(branch string) gitlab.MergeRequest {
	return gitlab.MergeRequest{IID: 77, State: "opened", SourceBranch: branch, TargetBranch: "main", WebURL: "https://gitlab.example/team/project/-/merge_requests/77"}
}
