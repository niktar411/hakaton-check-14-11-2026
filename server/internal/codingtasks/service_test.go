package codingtasks

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
	"hackathon/internal/llm"
)

func testService(store *fakeStore, model *fakeModel, worker *fakeWorker, gitLab *fakeGitLab) *Service {
	return NewService(store, model, worker, gitLab, Config{AdminUser: "root", AdminIP: "10.77.0.2", WorkerToken: "worker-test-token"})
}

func successfulGitLab(branch string) *fakeGitLab {
	return &fakeGitLab{byIID: map[int64]gitlab.MergeRequest{77: mergeRequest(branch)}}
}

func TestNewTaskAlwaysClarifiesAndOnlyConfirmationStartsWorker(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{clarification("Which parser inputs must be supported?")}}
	worker := &fakeWorker{}
	gitLab := successfulGitLab("coding/task-1")
	svc := testService(store, model, worker, gitLab)
	ctx := userContext(1, "alice")

	created, err := svc.Create(ctx, "Implement the parser", nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.Phase != PhaseAwaitingConfirmation || !strings.Contains(created.Clarification, "Which parser") {
		t.Fatalf("unexpected clarification state: %+v", created)
	}
	if calls := worker.operations(); len(calls) != 0 {
		t.Fatalf("worker called before confirmation: %v", calls)
	}
	if _, err := svc.Confirm(ctx, created.ID, "probably"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-explicit confirmation accepted: %v", err)
	}
	if calls := worker.operations(); len(calls) != 0 {
		t.Fatalf("worker called after ambiguous confirmation: %v", calls)
	}
	if len(model.requests) != 1 || !strings.Contains(model.requests[0].Messages[0].Content, "Maintains the parser") || !strings.Contains(model.requests[0].Messages[0].Content, "Direct and concise") {
		t.Fatal("personal context and tone missing from clarification system prompt")
	}

	model.add(
		toolResponse(call("check-1", ToolRunCheck, `{"check":"server-test"}`)),
		toolResponse(call("mr-1", ToolCreateMergeRequest, `{"taskId":1,"title":"Implement parser","description":"Implementation and server tests"}`)),
	)
	confirmed, err := svc.Confirm(ctx, created.ID, "understood")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Phase != PhaseAwaitingMerge || confirmed.MergeRequestIID == nil || *confirmed.MergeRequestIID != 77 || confirmed.MergeRequestURL == "" {
		t.Fatalf("merge request was not persisted: %+v", confirmed)
	}
	wantOperations := []string{ToolPrepareWorkspace, ToolRunCheck, ToolCreateMergeRequest}
	if got := worker.operations(); !reflect.DeepEqual(got, wantOperations) {
		t.Fatalf("worker operations = %v, want %v", got, wantOperations)
	}
	if len(model.requests) < 2 || !strings.Contains(model.requests[1].Messages[0].Content, "Maintains the parser") {
		t.Fatal("personal context missing from execution system prompt")
	}
	if _, err := svc.Confirm(ctx, created.ID, "understood"); !errors.Is(err, ErrConflict) {
		t.Fatalf("confirmed phase was reusable: %v", err)
	}
	if got := len(worker.operations()); got != len(wantOperations) {
		t.Fatalf("replayed confirmation caused worker calls: %d", got)
	}

	details, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var users, assistants, tools int
	for _, message := range details.Messages {
		switch message.Role {
		case "user":
			users++
		case "assistant":
			assistants++
		case "tool":
			tools++
		}
	}
	if users != 2 || assistants != 3 || tools != 3 {
		t.Fatalf("not every message was persisted: users=%d assistants=%d tools=%d", users, assistants, tools)
	}
}

func TestAnswerReentersClarificationAndNeedsFreshConfirmation(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{clarification("First question?"), clarification("Follow-up question?")}}
	worker := &fakeWorker{}
	gitLab := successfulGitLab("coding/task-1")
	svc := testService(store, model, worker, gitLab)
	ctx := userContext(1, "alice")
	created, err := svc.Create(ctx, "Add an endpoint", nil)
	if err != nil {
		t.Fatal(err)
	}
	answered, err := svc.Answer(ctx, created.ID, "Use the existing service pattern")
	if err != nil {
		t.Fatal(err)
	}
	if answered.Phase != PhaseAwaitingConfirmation || answered.Clarification != "Follow-up question?" {
		t.Fatalf("answer did not produce fresh clarification: %+v", answered)
	}
	if got := worker.operations(); len(got) != 0 {
		t.Fatalf("answer authorized workspace operations: %v", got)
	}
	details, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Messages) != 4 || details.Messages[2].Content != "Use the existing service pattern" {
		t.Fatalf("answer history not persisted: %+v", details.Messages)
	}

	model.add(
		toolResponse(call("check", ToolRunCheck, `{"check":"bun-check"}`)),
		toolResponse(call("mr", ToolCreateMergeRequest, `{"taskId":1,"title":"Add endpoint","description":"Done"}`)),
	)
	confirmed, err := svc.Confirm(ctx, created.ID, "подтверждаю")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Phase != PhaseAwaitingMerge {
		t.Fatalf("fresh explicit confirmation did not execute: %+v", confirmed)
	}
	if _, err := svc.Answer(ctx, created.ID, "too late"); !errors.Is(err, ErrConflict) {
		t.Fatalf("answer mutated a confirmed task: %v", err)
	}
}

func TestReworkCreatesIndependentClarifyingTask(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{clarification("Scope?")}}
	worker := &fakeWorker{}
	gitLab := successfulGitLab("coding/task-1")
	svc := testService(store, model, worker, gitLab)
	ctx := userContext(1, "alice")
	parent, err := svc.Create(ctx, "Initial implementation", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.add(
		toolResponse(call("check", ToolRunCheck, `{"check":"server-vet"}`)),
		toolResponse(call("mr", ToolCreateMergeRequest, `{"taskId":1,"title":"Initial","description":"Done"}`)),
	)
	parent, err = svc.Confirm(ctx, parent.ID, "confirm")
	if err != nil {
		t.Fatal(err)
	}
	before := len(worker.operations())
	model.add(clarification("What should change in the rework?"))
	child, err := svc.Rework(ctx, parent.ID, "Rework the error handling")
	if err != nil {
		t.Fatal(err)
	}
	if child.ID == parent.ID || child.ParentID == nil || *child.ParentID != parent.ID || child.Phase != PhaseAwaitingConfirmation {
		t.Fatalf("rework reused parent execution: parent=%+v child=%+v", parent, child)
	}
	if child.Branch != "" || child.MergeRequestIID != nil {
		t.Fatalf("rework inherited executable state: %+v", child)
	}
	if len(worker.operations()) != before {
		t.Fatal("rework called worker before its own confirmation")
	}
	storedParent, err := store.GetCodingTask(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedParent.Phase != PhaseAwaitingMerge {
		t.Fatalf("rework changed parent phase to %s", storedParent.Phase)
	}
}

func TestSecretAndOutsideWorkspaceToolsNeverReachWorker(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{clarification("Which file?")}}
	worker := &fakeWorker{}
	svc := testService(store, model, worker, successfulGitLab("coding/task-1"))
	ctx := userContext(1, "alice")
	created, err := svc.Create(ctx, "Read configuration", nil)
	if err != nil {
		t.Fatal(err)
	}
	model.add(
		toolResponse(
			call("secret-write", ToolWriteFile, `{"path":".env","content":"TOKEN=value"}`),
			call("outside", ToolWriteFile, `{"path":"../outside.go","content":"package outside"}`),
			call("secret-read", ToolReadFile, `{"path":".ssh/id_rsa"}`),
			call("secret-search", ToolSearchCode, `{"query":".env","path":""}`),
		),
		finalResponse("The requested operations are forbidden."),
	)
	result, err := svc.Confirm(ctx, created.ID, "confirm")
	if err != nil {
		t.Fatal(err)
	}
	if result.Phase != PhaseFailed {
		t.Fatalf("task with only rejected tools did not fail safely: %+v", result)
	}
	if got := worker.operations(); !reflect.DeepEqual(got, []string{ToolPrepareWorkspace}) {
		t.Fatalf("forbidden operation reached worker: %v", got)
	}
}

func TestFailedOrStaleChecksCannotCreateMergeRequest(t *testing.T) {
	for _, test := range []struct {
		name      string
		responses []llm.Response
		worker    *fakeWorker
		wantCalls []string
	}{
		{
			name: "failed check",
			responses: []llm.Response{
				toolResponse(call("check", ToolRunCheck, `{"check":"server-test"}`)),
				toolResponse(call("mr", ToolCreateMergeRequest, `{"taskId":1,"title":"Bad","description":"Must not be created"}`)),
				finalResponse("Unable to finish."),
			},
			worker: &fakeWorker{fn: func(operation string, _ ToolInput) (ToolOutput, error) {
				switch operation {
				case ToolPrepareWorkspace:
					return ToolOutput{OK: true}, nil
				case ToolRunCheck:
					return ToolOutput{OK: true, Passed: false, Output: "assertion failed"}, nil
				default:
					return ToolOutput{OK: true, MergeRequestIID: 77, MergeRequestURL: "https://gitlab.example/mr/77"}, nil
				}
			}},
			wantCalls: []string{ToolPrepareWorkspace, ToolRunCheck},
		},
		{
			name: "write invalidates earlier success",
			responses: []llm.Response{
				toolResponse(call("check", ToolRunCheck, `{"check":"client-build"}`)),
				toolResponse(call("write", ToolWriteFile, `{"path":"client/app.ts","content":"export const changed = true\n"}`)),
				toolResponse(call("mr", ToolCreateMergeRequest, `{"taskId":1,"title":"Stale","description":"Must not be created"}`)),
				finalResponse("A fresh check is required."),
			},
			worker:    &fakeWorker{},
			wantCalls: []string{ToolPrepareWorkspace, ToolRunCheck, ToolWriteFile},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			model := &fakeModel{responses: append([]llm.Response{clarification("Acceptance?")}, test.responses...)}
			gitLab := successfulGitLab("coding/task-1")
			svc := testService(store, model, test.worker, gitLab)
			created, err := svc.Create(userContext(1, "alice"), "Change code", nil)
			if err != nil {
				t.Fatal(err)
			}
			result, err := svc.Confirm(userContext(1, "alice"), created.ID, "confirm")
			if err != nil {
				t.Fatal(err)
			}
			if result.Phase != PhaseFailed || result.MergeRequestIID != nil {
				t.Fatalf("failed checks yielded merge request: %+v", result)
			}
			if got := test.worker.operations(); !reflect.DeepEqual(got, test.wantCalls) {
				t.Fatalf("worker operations = %v, want %v", got, test.wantCalls)
			}
			if len(gitLab.requests) != 0 {
				t.Fatal("GitLab was queried for a merge request that should not exist")
			}
		})
	}
}

func TestOwnershipAndAdminInspection(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{clarification("Question?")}}
	worker := &fakeWorker{}
	svc := testService(store, model, worker, successfulGitLab("coding/task-1"))
	alice := userContext(1, "alice")
	bob := userContext(2, "bob")
	created, err := svc.Create(alice, "Private task", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(bob, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner read task: %v", err)
	}
	if _, err := svc.Answer(bob, created.ID, "answer"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner answered task: %v", err)
	}
	if _, err := svc.Confirm(bob, created.ID, "confirm"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner confirmed task: %v", err)
	}
	if _, err := svc.Rework(bob, created.ID, "rework"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner reworked task: %v", err)
	}
	if len(worker.operations()) != 0 {
		t.Fatal("ownership failure reached worker")
	}
	if tasks, err := svc.List(bob, 0); err != nil || len(tasks) != 0 {
		t.Fatalf("other owner's list leaked tasks: %+v, %v", tasks, err)
	}
	if tasks, err := svc.List(alice, 0); err != nil || len(tasks) != 1 {
		t.Fatalf("owner list = %+v, %v", tasks, err)
	}
	if _, err := svc.AdminGet(bob, created.ID); err == nil {
		t.Fatal("member used admin inspection")
	}
	if tasks, err := svc.AdminList(adminContext(), 0); err != nil || len(tasks) != 1 {
		t.Fatalf("admin list = %+v, %v", tasks, err)
	}
	if details, err := svc.AdminGet(adminContext(), created.ID); err != nil || len(details.Messages) != 2 {
		t.Fatalf("admin detail = %+v, %v", details, err)
	}
	parentID := created.ID
	if _, err := svc.Create(bob, "Child of private task", &parentID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner attached a child task: %v", err)
	}
}

func TestInvalidClarificationFailsWithoutWorker(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{clarification("one", "two", "three", "four")}}
	worker := &fakeWorker{}
	svc := testService(store, model, worker, successfulGitLab("coding/task-1"))
	created, err := svc.Create(userContext(1, "alice"), "Task", nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.Phase != PhaseFailed || created.FailureReason != "model_invalid_clarification" {
		t.Fatalf("invalid clarification state: %+v", created)
	}
	if len(worker.operations()) != 0 {
		t.Fatal("invalid clarification reached worker")
	}
}

func TestMergeRequestMustMatchRealGitLabBranch(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{
		clarification("Question?"),
		toolResponse(call("check", ToolRunCheck, `{"check":"server-test"}`)),
		toolResponse(call("mr", ToolCreateMergeRequest, `{"taskId":1,"title":"Title","description":"Description"}`)),
	}}
	worker := &fakeWorker{}
	wrong := mergeRequest("coding/task-1")
	wrong.SourceBranch = "somebody/else"
	gitLab := &fakeGitLab{byIID: map[int64]gitlab.MergeRequest{77: wrong}}
	svc := testService(store, model, worker, gitLab)
	ctx := userContext(1, "alice")
	created, err := svc.Create(ctx, "Task", nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Confirm(ctx, created.ID, "confirm")
	if err != nil {
		t.Fatal(err)
	}
	if result.Phase != PhaseFailed || result.FailureReason != "merge_request_verification_failed" || result.MergeRequestIID != nil {
		t.Fatalf("unverified merge request accepted: %+v", result)
	}
}

func TestStrictToolSchemas(t *testing.T) {
	valid := llm.ToolCall{ID: "1", Type: "function", Function: llm.FunctionCall{Name: ToolWriteFile, Arguments: `{"path":"main.go","content":"package main\n"}`}}
	if _, err := parseToolCall(1, valid); err != nil {
		t.Fatalf("valid write rejected: %v", err)
	}
	started, err := parseToolCall(1, call("1", ToolStartPlannerBlock, `{"plannerBlockId":7}`))
	if err != nil || started.PlannerBlockID != 7 {
		t.Fatalf("valid planner start rejected: %#v, %v", started, err)
	}
	for _, test := range []struct {
		name string
		call llm.ToolCall
	}{
		{"unknown field", call("1", ToolWriteFile, `{"path":"main.go","content":"x","command":"rm"}`)},
		{"trailing JSON", call("1", ToolWriteFile, `{"path":"main.go","content":"x"}{}`)},
		{"arbitrary check", call("1", ToolRunCheck, `{"check":"shell"}`)},
		{"wrong task", call("1", ToolCreateMergeRequest, `{"taskId":2,"title":"x","description":"x"}`)},
		{"absolute path", call("1", ToolReadFile, `{"path":"/etc/passwd"}`)},
		{"secret path", call("1", ToolReadFile, `{"path":"config/private.key"}`)},
		{"workspace repreparation", call("1", ToolPrepareWorkspace, `{"taskId":1,"parentTaskId":0}`)},
		{"unknown operation", call("1", "execute_command", `{"command":"id"}`)},
		{"planner start without id", call("1", ToolStartPlannerBlock, `{}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseToolCall(1, test.call); err == nil {
				t.Fatal("invalid tool call accepted")
			}
		})
	}
}

func TestParentPreparesFromParentOnlyAfterConfirmation(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{clarification("Parent question?"), clarification("Child question?")}}
	worker := &fakeWorker{}
	gitLab := successfulGitLab("coding/task-2")
	svc := testService(store, model, worker, gitLab)
	ctx := userContext(1, "alice")
	parent, err := svc.Create(ctx, "Parent", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.Rework(ctx, parent.ID, "Child")
	if err != nil {
		t.Fatal(err)
	}
	model.add(
		toolResponse(call("check", ToolRunCheck, `{"check":"server-vet"}`)),
		toolResponse(call("mr", ToolCreateMergeRequest, `{"taskId":2,"title":"Child","description":"Done"}`)),
	)
	if _, err := svc.Confirm(ctx, child.ID, "confirm"); err != nil {
		t.Fatal(err)
	}
	worker.mu.Lock()
	defer worker.mu.Unlock()
	if len(worker.calls) == 0 || worker.calls[0].operation != ToolPrepareWorkspace || worker.calls[0].input.ParentTaskID != parent.ID {
		t.Fatalf("parent workspace was not passed safely: %+v", worker.calls)
	}
}

func TestReworkDoesNotCopyParentMessages(t *testing.T) {
	store := newFakeStore()
	model := &fakeModel{responses: []llm.Response{clarification("Parent?"), clarification("Child?")}}
	svc := testService(store, model, &fakeWorker{}, successfulGitLab("coding/task-1"))
	ctx := userContext(1, "alice")
	parent, err := svc.Create(ctx, "Parent request", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.Rework(ctx, parent.ID, "Child request")
	if err != nil {
		t.Fatal(err)
	}
	details, err := svc.Get(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Messages) != 2 || details.Messages[0].Content != "Child request" {
		t.Fatalf("child inherited parent conversation: %+v", details.Messages)
	}
	stored, err := store.GetOwnedCodingTask(t.Context(), db.GetOwnedCodingTaskParams{ID: parent.ID, AccountID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if stored.ParentID != (pgtype.Int8{}) {
		t.Fatal("new parent unexpectedly has a parent")
	}
}
