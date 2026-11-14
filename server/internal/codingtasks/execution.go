package codingtasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "hackathon/internal/db/sqlc"
	"hackathon/internal/gitlab"
	"hackathon/internal/identity"
	"hackathon/internal/llm"
	"hackathon/internal/plannerblocks"
)

const (
	maxModelIterations = 16
	maxToolCalls       = 48
	maxCallsPerTurn    = 4
	maxHistoryBytes    = 300000
)

type toolAudit struct {
	CallID    string     `json:"callId"`
	Operation string     `json:"operation"`
	Input     *ToolInput `json:"input,omitempty"`
	Output    ToolOutput `json:"output"`
}

func (s *Service) execute(ctx context.Context, task db.CodingTask, account db.LocalAccount) (db.CodingTask, error) {
	if s.worker == nil {
		return s.fail(ctx, task.ID, "worker_unavailable")
	}
	if s.gitLab == nil {
		return s.fail(ctx, task.ID, "gitlab_unavailable")
	}
	execCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()

	parentID := int64(0)
	if task.ParentID.Valid {
		parentID = task.ParentID.Int64
	}
	prepareInput := ToolInput{TaskID: task.ID, ParentTaskID: parentID}
	prepareOutput, prepareErr := s.callWorker(execCtx, ToolPrepareWorkspace, prepareInput)
	if prepareErr != nil {
		prepareOutput = ToolOutput{OK: false, Error: "worker_operation_failed"}
	}
	if _, err := s.persistTool(execCtx, task.ID, "workspace", ToolPrepareWorkspace, &prepareInput, prepareOutput); err != nil {
		return db.CodingTask{}, err
	}
	if prepareErr != nil || !prepareOutput.OK {
		return s.fail(execCtx, task.ID, "workspace_preparation_failed")
	}

	history, err := s.store.ListCodingMessages(execCtx, db.ListCodingMessagesParams{TaskID: task.ID, Limit: 500})
	if err != nil {
		return db.CodingTask{}, fmt.Errorf("load execution history: %w", err)
	}
	principal, principalErr := identity.FromContext(ctx)
	adminTools := principalErr == nil && identity.IsAdmin(principal, s.config.AdminUser, s.config.AdminIP)
	var owned []plannerblocks.PlannerBlock
	if s.adminPlanner != nil {
		owned, err = s.adminPlanner.ListOwned(execCtx, account.Username)
		if err != nil {
			return db.CodingTask{}, fmt.Errorf("load owned planner blocks: %w", err)
		}
	}
	messages := []llm.Message{{Role: "system", Content: executionPrompt(account, task.ID, adminTools, owned)}}
	historyBytes := 0
	for _, row := range history {
		if row.Role != "user" && row.Role != "assistant" {
			continue
		}
		historyBytes += len(row.Content)
		if historyBytes > maxHistoryBytes {
			return s.fail(execCtx, task.ID, "conversation_limit_exceeded")
		}
		messages = append(messages, llm.Message{Role: row.Role, Content: row.Content})
	}

	checks := make(map[string]bool)
	totalCalls := 0
	for iteration := 0; iteration < maxModelIterations; iteration++ {
		response, modelErr := s.model.Complete(execCtx, llm.Request{Messages: messages, Tools: modelTools(adminTools), MaxTokens: 4096, Actor: account.Username, RequestID: fmt.Sprintf("coding-execute-%d-%d", task.ID, iteration), Session: fmt.Sprintf("coding-%d", task.ID)})
		if modelErr != nil {
			return s.fail(execCtx, task.ID, executionFailure(execCtx, "model_execution_failed"))
		}
		message := response.Message
		if len(message.Content) > 65536 || len(message.ToolCalls) > maxCallsPerTurn {
			return s.fail(execCtx, task.ID, "model_response_limit_exceeded")
		}
		if strings.TrimSpace(message.Content) == "" && len(message.ToolCalls) == 0 {
			return s.fail(execCtx, task.ID, "model_empty_response")
		}
		if _, err := s.store.AddCodingMessage(execCtx, db.AddCodingMessageParams{TaskID: task.ID, Role: "assistant", Content: message.Content, ToolName: assistantToolName(message.ToolCalls)}); err != nil {
			return db.CodingTask{}, fmt.Errorf("store execution assistant message: %w", err)
		}
		messages = append(messages, message)
		if len(message.ToolCalls) == 0 {
			return s.fail(execCtx, task.ID, "model_finished_without_merge_request")
		}
		totalCalls += len(message.ToolCalls)
		if totalCalls > maxToolCalls {
			return s.fail(execCtx, task.ID, "tool_call_limit_exceeded")
		}
		if duplicateToolCallIDs(message.ToolCalls) {
			return s.fail(execCtx, task.ID, "invalid_tool_call_id")
		}

		for _, call := range message.ToolCalls {
			callID := call.ID
			operation := call.Function.Name
			input, parseErr := parseToolCall(task.ID, call)
			if parseErr != nil {
				content, err := s.persistTool(execCtx, task.ID, callID, operation, nil, ToolOutput{OK: false, Error: "invalid_tool_arguments"})
				if err != nil {
					return db.CodingTask{}, err
				}
				messages = append(messages, llm.Message{Role: "tool", ToolCallID: callID, Name: operation, Content: content})
				continue
			}
			if operation == ToolStartPlannerBlock {
				output := ToolOutput{OK: false, Error: "planner_unavailable"}
				if s.adminPlanner != nil {
					if _, startErr := s.adminPlanner.StartForOwner(execCtx, input.PlannerBlockID, account.Username); startErr != nil {
						output = ToolOutput{OK: false, Error: "planner_start_failed"}
					} else {
						output = ToolOutput{OK: true}
					}
				}
				content, err := s.persistTool(execCtx, task.ID, callID, operation, &input, output)
				if err != nil {
					return db.CodingTask{}, err
				}
				messages = append(messages, llm.Message{Role: "tool", ToolCallID: callID, Name: operation, Content: content})
				continue
			}
			if operation == ToolCreateMergeRequest && !checksPassed(checks) {
				content, err := s.persistTool(execCtx, task.ID, callID, operation, &input, ToolOutput{OK: false, Error: "checks_not_passed"})
				if err != nil {
					return db.CodingTask{}, err
				}
				messages = append(messages, llm.Message{Role: "tool", ToolCallID: callID, Name: operation, Content: content})
				continue
			}

			var output ToolOutput
			var operationErr error
			if isAdminOperation(operation) {
				output, operationErr = s.callAdminOperation(execCtx, operation, input)
			} else {
				output, operationErr = s.callWorker(execCtx, operation, input)
			}
			if operationErr != nil {
				output = ToolOutput{OK: false, Error: "worker_operation_failed"}
			}
			if !output.OK && output.Error == "" {
				output.Error = "operation_failed"
			}
			if output.OK {
				output.Error = ""
			}
			if operation == ToolRunCheck {
				checks[input.Check] = output.OK && output.Passed
				if output.OK && !output.Passed {
					output.Error = "check_failed"
				}
			}
			if operation == ToolWriteFile && output.OK {
				checks = make(map[string]bool)
			}
			content, err := s.persistTool(execCtx, task.ID, callID, operation, &input, output)
			if err != nil {
				return db.CodingTask{}, err
			}
			messages = append(messages, llm.Message{Role: "tool", ToolCallID: callID, Name: operation, Content: content})

			if operation == ToolCreateMergeRequest && output.OK {
				mergeRequest, verifyErr := s.verifyMergeRequest(execCtx, output.MergeRequestIID, task.Branch)
				if verifyErr != nil {
					return s.fail(execCtx, task.ID, "merge_request_verification_failed")
				}
				updated, storeErr := s.store.SetCodingTaskMergeRequest(execCtx, db.SetCodingTaskMergeRequestParams{
					ID: task.ID, MergeRequestIid: pgtype.Int8{Int64: mergeRequest.IID, Valid: true}, MergeRequestUrl: mergeRequest.WebURL,
				})
				if storeErr != nil {
					failed, failErr := s.fail(execCtx, task.ID, "merge_request_persistence_failed")
					if failErr != nil {
						return db.CodingTask{}, fmt.Errorf("store merge request: %w", storeErr)
					}
					return failed, nil
				}
				s.notifyMergeRequestCreated(execCtx, task.ID, task.Branch, mergeRequest.WebURL)
				return updated, nil
			}
			if execCtx.Err() != nil {
				return s.fail(execCtx, task.ID, "execution_time_limit_exceeded")
			}
		}
	}
	return s.fail(execCtx, task.ID, "model_iteration_limit_exceeded")
}

// notifyMergeRequestCreated reports a verified merge request to the chat
// notification sink. The sink deduplicates events per task, so repeated tool
// calls for the same task never create duplicate notifications.
func (s *Service) notifyMergeRequestCreated(ctx context.Context, taskID int64, branch, mergeRequestURL string) {
	s.locksMu.Lock()
	notifier := s.notifier
	s.locksMu.Unlock()
	if notifier == nil {
		return
	}
	notifier.MergeRequestCreated(ctx, taskID, branch, mergeRequestURL)
}

func executionFailure(ctx context.Context, fallback string) string {
	if ctx.Err() != nil {
		return "execution_time_limit_exceeded"
	}
	return fallback
}

func duplicateToolCallIDs(calls []llm.ToolCall) bool {
	seen := make(map[string]struct{}, len(calls))
	for _, call := range calls {
		if call.ID == "" || len(call.ID) > 200 || strings.ContainsAny(call.ID, "\r\n\x00") {
			return true
		}
		if _, ok := seen[call.ID]; ok {
			return true
		}
		seen[call.ID] = struct{}{}
	}
	return false
}

func checksPassed(checks map[string]bool) bool {
	if len(checks) == 0 {
		return false
	}
	for _, passed := range checks {
		if !passed {
			return false
		}
	}
	return true
}

func (s *Service) callWorker(ctx context.Context, operation string, input ToolInput) (ToolOutput, error) {
	if err := validateToolInput(operation, input); err != nil {
		return ToolOutput{}, err
	}
	output, err := s.worker.Execute(ctx, operation, input)
	if err != nil {
		return ToolOutput{}, err
	}
	if len(output.Content) > 1<<20 || len(output.Output) > 1<<20 || len(output.Files) > 10000 || len(output.Matches) > 10000 {
		return ToolOutput{}, fmt.Errorf("worker output exceeds limits")
	}
	if err := validateToolOutput(operation, output); err != nil {
		return ToolOutput{}, err
	}
	return s.redactWorkerOutput(output), nil
}

func (s *Service) redactWorkerOutput(output ToolOutput) ToolOutput {
	secret := s.config.WorkerToken
	if secret == "" {
		return output
	}
	redact := func(value string) string { return strings.ReplaceAll(value, secret, "[redacted]") }
	output.Content = redact(output.Content)
	output.Output = redact(output.Output)
	output.Error = redact(output.Error)
	for i := range output.Files {
		output.Files[i] = redact(output.Files[i])
	}
	for i := range output.Matches {
		output.Matches[i].Text = redact(output.Matches[i].Text)
	}
	return output
}

func (s *Service) persistTool(ctx context.Context, taskID int64, callID, operation string, input *ToolInput, output ToolOutput) (string, error) {
	if len(callID) > 200 {
		callID = callID[:200]
	}
	data, err := json.Marshal(toolAudit{CallID: callID, Operation: operation, Input: input, Output: output})
	if err != nil {
		return "", fmt.Errorf("encode tool audit: %w", err)
	}
	if len(data) > (2 << 20) {
		return "", fmt.Errorf("tool audit exceeds limits")
	}
	toolName := operation
	if toolName == "" || len(toolName) > 100 {
		toolName = "invalid"
	}
	if _, err := s.store.AddCodingMessage(ctx, db.AddCodingMessageParams{TaskID: taskID, Role: "tool", Content: string(data), ToolName: toolName}); err != nil {
		return "", fmt.Errorf("store tool message: %w", err)
	}
	return string(data), nil
}

func (s *Service) verifyMergeRequest(ctx context.Context, iid int64, branch string) (gitlab.MergeRequest, error) {
	mergeRequest, err := s.gitLab.MergeRequestByIID(ctx, iid)
	if err != nil {
		return gitlab.MergeRequest{}, err
	}
	parsed, err := url.Parse(mergeRequest.WebURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
		return gitlab.MergeRequest{}, fmt.Errorf("invalid merge request URL")
	}
	if mergeRequest.IID != iid || mergeRequest.SourceBranch != branch || mergeRequest.State != "opened" {
		return gitlab.MergeRequest{}, fmt.Errorf("merge request does not match task")
	}
	return mergeRequest, nil
}

func parseToolCall(taskID int64, call llm.ToolCall) (ToolInput, error) {
	if call.Type != "function" || call.Function.Name == ToolPrepareWorkspace {
		return ToolInput{}, ErrInvalid
	}
	switch call.Function.Name {
	case ToolListFiles:
		var args struct {
			Path *string `json:"path"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.Path == nil {
			return ToolInput{}, ErrInvalid
		}
		return validParsedInput(call.Function.Name, ToolInput{TaskID: taskID, Path: *args.Path})
	case ToolReadFile:
		var args struct {
			Path *string `json:"path"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.Path == nil {
			return ToolInput{}, ErrInvalid
		}
		return validParsedInput(call.Function.Name, ToolInput{TaskID: taskID, Path: *args.Path})
	case ToolSearchCode:
		var args struct {
			Query *string `json:"query"`
			Path  *string `json:"path"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.Query == nil || args.Path == nil {
			return ToolInput{}, ErrInvalid
		}
		return validParsedInput(call.Function.Name, ToolInput{TaskID: taskID, Query: *args.Query, Path: *args.Path})
	case ToolWriteFile:
		var args struct {
			Path    *string `json:"path"`
			Content *string `json:"content"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.Path == nil || args.Content == nil {
			return ToolInput{}, ErrInvalid
		}
		return validParsedInput(call.Function.Name, ToolInput{TaskID: taskID, Path: *args.Path, Content: *args.Content})
	case ToolRunCheck:
		var args struct {
			Check *string `json:"check"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.Check == nil {
			return ToolInput{}, ErrInvalid
		}
		return validParsedInput(call.Function.Name, ToolInput{TaskID: taskID, Check: *args.Check})
	case ToolCreateMergeRequest:
		var args struct {
			TaskID      *int64  `json:"taskId"`
			Title       *string `json:"title"`
			Description *string `json:"description"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.TaskID == nil || *args.TaskID != taskID || args.Title == nil || args.Description == nil {
			return ToolInput{}, ErrInvalid
		}
		return validParsedInput(call.Function.Name, ToolInput{TaskID: taskID, Title: *args.Title, Description: *args.Description})
	case ToolStartPlannerBlock:
		var args struct {
			PlannerBlockID *int64 `json:"plannerBlockId"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.PlannerBlockID == nil || *args.PlannerBlockID < 1 {
			return ToolInput{}, ErrInvalid
		}
		return ToolInput{TaskID: taskID, PlannerBlockID: *args.PlannerBlockID}, nil
	case ToolCreatePlannerBlock:
		var args struct {
			Title       *string `json:"title"`
			Description string  `json:"description"`
			OwnerID     *int64  `json:"ownerId"`
			Position    *int32  `json:"position"`
			Branch      string  `json:"branch"`
			MRIID       *int64  `json:"mrIid"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.Title == nil || args.OwnerID == nil {
			return ToolInput{}, ErrInvalid
		}
		return ToolInput{TaskID: taskID, Title: *args.Title, Description: args.Description, OwnerID: *args.OwnerID, Position: args.Position, Branch: args.Branch, MRIID: args.MRIID}, nil
	case ToolOperateService:
		var args struct {
			Action *string `json:"action"`
			Target *string `json:"target"`
		}
		if err := decodeArguments(call.Function.Arguments, &args); err != nil || args.Action == nil || args.Target == nil || !validAIOperation(*args.Action, *args.Target) {
			return ToolInput{}, ErrInvalid
		}
		return ToolInput{TaskID: taskID, Action: *args.Action, Target: *args.Target}, nil
	default:
		return ToolInput{}, ErrInvalid
	}
}

func validParsedInput(operation string, input ToolInput) (ToolInput, error) {
	if err := validateToolInput(operation, input); err != nil {
		return ToolInput{}, err
	}
	return input, nil
}

func decodeArguments(raw string, destination any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 300000 || !strings.HasPrefix(raw, "{") {
		return ErrInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return ErrInvalid
	}
	if err := requireJSONEnd(decoder); err != nil {
		return ErrInvalid
	}
	return nil
}
